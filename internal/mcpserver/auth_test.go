package mcpserver

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// --- LoadOrCreateToken ---

func TestLoadOrCreateToken(t *testing.T) {
	t.Parallel()

	t.Run("creates_new", func(t *testing.T) {
		dir := t.TempDir()
		tok, err := LoadOrCreateToken(dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(tok) != 64 { // 32 bytes = 64 hex chars
			t.Errorf("token length = %d, want 64", len(tok))
		}
		info, err := os.Stat(filepath.Join(dir, tokenFile))
		if err != nil {
			t.Fatal(err)
		}
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Errorf("perm = %o, want 600", perm)
		}
	})

	t.Run("reads_existing", func(t *testing.T) {
		dir := t.TempDir()
		want := "my-secret-token"
		if err := os.WriteFile(filepath.Join(dir, tokenFile), []byte(want+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := LoadOrCreateToken(dir)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("trims_whitespace", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, tokenFile), []byte("  tok  \n"), 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := LoadOrCreateToken(dir)
		if err != nil {
			t.Fatal(err)
		}
		if got != "tok" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("empty_file_regenerates", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, tokenFile), []byte("\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		tok, err := LoadOrCreateToken(dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(tok) != 64 {
			t.Errorf("expected new 64-char token, got %q", tok)
		}
	})
}

// --- bearerAuthMiddleware ---

func TestBearerAuthMiddleware(t *testing.T) {
	t.Parallel()

	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	tests := []struct {
		name       string
		token      string
		authHeader string
		wantCode   int
	}{
		{"empty_token_passthrough", "", "", 200},
		{"valid_token", "secret", "Bearer secret", 200},
		{"wrong_token", "secret", "Bearer wrong", 401},
		{"missing_header", "secret", "", 401},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := bearerAuthMiddleware(tt.token, ok)
			req := httptest.NewRequest("GET", "/", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tt.wantCode {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantCode)
			}
		})
	}
}
