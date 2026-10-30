package headless

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"calabrass-tui/internal/api"
)

func TestProviderFromEnvTurnsOnTheProviderWithItsKey(t *testing.T) {
	var gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.Method + " " + r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = w.Write([]byte(`{"providers":[]}`))
	}))
	defer srv.Close()

	env := map[string]string{"CALABRASS_PROVIDER": "groq", "CALABRASS_API_KEY": "gsk-123", "CALABRASS_MODEL": "llama"}
	opts := Options{}
	err := ProviderFromEnv(context.Background(), api.New(strings.TrimPrefix(srv.URL, "http://")), func(k string) string { return env[k] }, &opts)
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "PUT /api/providers/groq" || gotBody["apiKey"] != "gsk-123" || gotBody["enabled"] != true {
		t.Fatalf("%s %v", gotPath, gotBody)
	}
	if opts.Model != "groq/llama" {
		t.Fatalf("model = %q", opts.Model)
	}
}

func TestProviderFromEnvLeavesAnExplicitModelAndDoesNothingWithoutBoth(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
	defer srv.Close()
	c := api.New(strings.TrimPrefix(srv.URL, "http://"))

	// Only one of the two set: nothing happens.
	opts := Options{}
	if err := ProviderFromEnv(context.Background(), c, func(k string) string {
		return map[string]string{"CALABRASS_PROVIDER": "groq"}[k]
	}, &opts); err != nil || called {
		t.Fatalf("acted on half a configuration: err=%v called=%v", err, called)
	}

	// An explicit --model wins over CALABRASS_MODEL.
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{"providers":[]}`)) }))
	defer srv2.Close()
	opts = Options{Model: "openai/gpt"}
	_ = ProviderFromEnv(context.Background(), api.New(strings.TrimPrefix(srv2.URL, "http://")), func(k string) string {
		return map[string]string{"CALABRASS_PROVIDER": "groq", "CALABRASS_API_KEY": "k", "CALABRASS_MODEL": "llama"}[k]
	}, &opts)
	if opts.Model != "openai/gpt" {
		t.Fatalf("an explicit model was overridden: %q", opts.Model)
	}
}
