package headless

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"

	"calabrass-tui/internal/api"
	"calabrass-tui/internal/session"
	"calabrass-tui/internal/settings"
)

// Main is `cal run`: it reads the command line, signs in, runs the task, and
// returns the process exit code.
func Main(args []string, stdin *os.File, stdout, stderr io.Writer) int {
	piped := false
	if info, err := stdin.Stat(); err == nil {
		piped = info.Mode()&os.ModeCharDevice == 0
	}
	opts, err := Parse(args, stdin, piped)
	if err != nil {
		fmt.Fprintln(stderr, "calabrass:", err)
		return ExitUsage
	}

	st, err := settings.Load()
	if err != nil {
		st = settings.Settings{}
	}
	if v := os.Getenv("CALABRASS_ADDR"); v != "" {
		st.Addr = v
	}
	client := api.New(st.Addr)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if _, err := session.Ensure(ctx, client, &st); err != nil {
		var unreachable *session.ErrUnreachable
		if errors.As(err, &unreachable) {
			fmt.Fprintln(stderr, "calabrass: no Calabrass host answers. Open the desktop app, or run `cal` once so it starts one.")
		} else {
			fmt.Fprintln(stderr, "calabrass: could not sign in:", err)
		}
		return ExitSetup
	}
	_ = st.Save()

	// A pipeline has no screen to add a provider on, so it can say which one and
	// bring its key in the environment.
	if err := ProviderFromEnv(ctx, client, os.Getenv, &opts); err != nil {
		fmt.Fprintln(stderr, "calabrass:", err)
		return ExitSetup
	}

	return Run(ctx, client, opts, stdout, stderr).Exit
}

// ProviderFromEnv turns on a model provider from the environment, for a machine
// no one will ever sit at (CI, a container, a server):
//
//	CALABRASS_PROVIDER   a provider id, for example openai, groq, google
//	CALABRASS_API_KEY    its key
//	CALABRASS_MODEL      the model; becomes the run's model when --model is not given
//
// Nothing happens unless CALABRASS_PROVIDER and CALABRASS_API_KEY are both set.
// The key is handed to the host, which stores it protected, and is not written
// anywhere by this program.
func ProviderFromEnv(ctx context.Context, c *api.Client, getenv func(string) string, opts *Options) error {
	id := strings.TrimSpace(getenv("CALABRASS_PROVIDER"))
	key := strings.TrimSpace(getenv("CALABRASS_API_KEY"))
	if id == "" || key == "" {
		return nil
	}
	enabled := true
	if _, err := c.UpdateProvider(ctx, id, api.ProviderUpdate{APIKey: &key, Enabled: &enabled}); err != nil {
		return fmt.Errorf("could not set up provider %q from CALABRASS_PROVIDER / CALABRASS_API_KEY: %w", id, err)
	}
	if opts.Model == "" {
		if model := strings.TrimSpace(getenv("CALABRASS_MODEL")); model != "" {
			opts.Model = id + "/" + model
		}
	}
	return nil
}
