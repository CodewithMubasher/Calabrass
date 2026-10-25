// Package session gets the TUI a valid backend session without any screen of
// its own: it reuses a saved token, signs in with credentials from the
// environment, or creates a local account for itself.
package session

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"strings"

	"calabrass-tui/internal/api"
	"calabrass-tui/internal/settings"
)

// Environment variables that name an existing account to sign in with.
const (
	EnvEmail    = "CALABRASS_EMAIL"
	EnvPassword = "CALABRASS_PASSWORD"
)

// ErrUnreachable means the backend did not answer at all.
type ErrUnreachable struct{ Err error }

func (e *ErrUnreachable) Error() string { return e.Err.Error() }
func (e *ErrUnreachable) Unwrap() error { return e.Err }

// Ensure leaves c with a working token and returns the account. It updates st
// (the caller saves it) when it signs in or creates the local account.
func Ensure(ctx context.Context, c *api.Client, st *settings.Settings) (api.User, error) {
	if err := c.Health(ctx); err != nil {
		return api.User{}, &ErrUnreachable{Err: err}
	}

	// 1. A saved token that still works.
	if st.Token != "" {
		c.Token = st.Token
		user, err := c.Me(ctx)
		if err == nil {
			return user, nil
		}
		if !api.IsUnauthorized(err) {
			return api.User{}, wrap(err)
		}
		c.Token, st.Token = "", ""
	}

	// 2. An account named in the environment.
	email := strings.TrimSpace(os.Getenv(EnvEmail))
	password := os.Getenv(EnvPassword)
	if email != "" && password != "" {
		sess, err := c.Login(ctx, email, password)
		if err != nil {
			return api.User{}, wrap(err)
		}
		return adopt(c, st, sess), nil
	}

	// 3. The local account this TUI made earlier.
	if st.Email != "" && st.Password != "" {
		sess, err := c.Login(ctx, st.Email, st.Password)
		if err == nil {
			return adopt(c, st, sess), nil
		}
		if !api.IsAPIError(err) {
			return api.User{}, wrap(err)
		}
		// The account is gone (fresh database): make a new one below.
		st.Email, st.Password = "", ""
	}

	// 4. Create a local account.
	suffix, err := randomHex(4)
	if err != nil {
		return api.User{}, err
	}
	pass, err := randomHex(16)
	if err != nil {
		return api.User{}, err
	}
	email = "tui-" + suffix + "@calabrass.local"
	sess, err := c.SignUp(ctx, "Calabrass TUI", email, pass)
	if err != nil {
		return api.User{}, wrap(err)
	}
	st.Email, st.Password = email, pass
	return adopt(c, st, sess), nil
}

func adopt(c *api.Client, st *settings.Settings, sess api.Session) api.User {
	c.Token = sess.Token
	st.Token = sess.Token
	return sess.User
}

// wrap marks transport failures as "unreachable" and leaves backend answers alone.
func wrap(err error) error {
	if api.IsAPIError(err) {
		return err
	}
	var already *ErrUnreachable
	if errors.As(err, &already) {
		return err
	}
	return &ErrUnreachable{Err: err}
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
