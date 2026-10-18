package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/CodewithMubasher/Calabrass/backend/internal/store"
)

func TestAuthSignUpLoginAndSession(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	server := httptest.NewServer(NewServer(Deps{Store: st, Logger: slog.Default()}))
	t.Cleanup(server.Close)

	request := func(method, path, token string, body any) (*http.Response, map[string]any) {
		t.Helper()
		var reader io.Reader
		if body != nil {
			raw, err := json.Marshal(body)
			if err != nil {
				t.Fatalf("marshal body: %v", err)
			}
			reader = bytes.NewReader(raw)
		}
		req, err := http.NewRequest(method, server.URL+path, reader)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		defer func() { _ = res.Body.Close() }()

		payload := map[string]any{}
		if res.StatusCode != http.StatusNoContent {
			if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
				t.Fatalf("%s %s: decode response: %v", method, path, err)
			}
		}
		return res, payload
	}

	signUp := map[string]string{
		"name":     "Ada Lovelace",
		"email":    "ada@example.com",
		"password": "a long enough password",
	}

	res, body := request(http.MethodPost, "/api/auth/signup", "", signUp)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("signup status = %d, want 201 (%v)", res.StatusCode, body)
	}
	token, _ := body["token"].(string)
	if token == "" {
		t.Fatal("signup returned no token")
	}
	user, _ := body["user"].(map[string]any)
	if user["email"] != signUp["email"] || user["name"] != signUp["name"] {
		t.Fatalf("signup user = %v", user)
	}

	res, body = request(http.MethodGet, "/api/auth/me", token, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("me status = %d, want 200 (%v)", res.StatusCode, body)
	}
	if me, _ := body["user"].(map[string]any); me["email"] != signUp["email"] {
		t.Fatalf("me user = %v", body["user"])
	}

	res, body = request(http.MethodPost, "/api/auth/signup", "", signUp)
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate signup status = %d, want 409 (%v)", res.StatusCode, body)
	}

	res, body = request(http.MethodPost, "/api/auth/login", "", map[string]string{
		"email":    signUp["email"],
		"password": "not the password",
	})
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong password status = %d, want 401 (%v)", res.StatusCode, body)
	}

	res, body = request(http.MethodPost, "/api/auth/login", "", map[string]string{
		"email":    signUp["email"],
		"password": signUp["password"],
	})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d, want 200 (%v)", res.StatusCode, body)
	}
	loginToken, _ := body["token"].(string)
	if loginToken == "" || loginToken == token {
		t.Fatalf("login token = %q, want a new token", loginToken)
	}

	res, _ = request(http.MethodPost, "/api/auth/logout", loginToken, nil)
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("logout status = %d, want 204", res.StatusCode)
	}

	res, body = request(http.MethodGet, "/api/auth/me", loginToken, nil)
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("me after logout status = %d, want 401 (%v)", res.StatusCode, body)
	}
}
