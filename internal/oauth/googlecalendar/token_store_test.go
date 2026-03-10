package googlecalendar

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

func TestSaveTokenPersistsToDisk(t *testing.T) {
	t.Setenv("GOOGLE_OAUTH_TOKEN_PATH", filepath.Join(t.TempDir(), "google_oauth_token.json"))
	resetTokenCache()

	original := &oauth2.Token{
		AccessToken:  "access-1",
		RefreshToken: "refresh-1",
		TokenType:    "Bearer",
		Expiry:       time.Unix(1700000000, 0).UTC(),
	}
	if err := SaveToken(original); err != nil {
		t.Fatalf("SaveToken() error = %v", err)
	}

	resetTokenCache()
	got, err := LoadToken()
	if err != nil {
		t.Fatalf("LoadToken() error = %v", err)
	}
	if got.AccessToken != original.AccessToken {
		t.Fatalf("AccessToken = %q, want %q", got.AccessToken, original.AccessToken)
	}
	if got.RefreshToken != original.RefreshToken {
		t.Fatalf("RefreshToken = %q, want %q", got.RefreshToken, original.RefreshToken)
	}
}

func TestSaveTokenPreservesRefreshToken(t *testing.T) {
	t.Setenv("GOOGLE_OAUTH_TOKEN_PATH", filepath.Join(t.TempDir(), "google_oauth_token.json"))
	resetTokenCache()

	if err := SaveToken(&oauth2.Token{
		AccessToken:  "access-1",
		RefreshToken: "refresh-1",
		TokenType:    "Bearer",
		Expiry:       time.Unix(1700000000, 0).UTC(),
	}); err != nil {
		t.Fatalf("SaveToken(initial) error = %v", err)
	}

	if err := SaveToken(&oauth2.Token{
		AccessToken: "access-2",
		TokenType:   "Bearer",
		Expiry:      time.Unix(1700003600, 0).UTC(),
	}); err != nil {
		t.Fatalf("SaveToken(updated) error = %v", err)
	}

	resetTokenCache()
	got, err := LoadToken()
	if err != nil {
		t.Fatalf("LoadToken() error = %v", err)
	}
	if got.AccessToken != "access-2" {
		t.Fatalf("AccessToken = %q, want %q", got.AccessToken, "access-2")
	}
	if got.RefreshToken != "refresh-1" {
		t.Fatalf("RefreshToken = %q, want %q", got.RefreshToken, "refresh-1")
	}
}

func TestLoadTokenMissingFile(t *testing.T) {
	t.Setenv("GOOGLE_OAUTH_TOKEN_PATH", filepath.Join(t.TempDir(), "missing.json"))
	resetTokenCache()

	_, err := LoadToken()
	if err == nil {
		t.Fatal("LoadToken() error = nil, want error")
	}
	if !os.IsNotExist(err) && err != errNoToken {
		t.Fatalf("LoadToken() error = %v, want errNoToken", err)
	}
}

func resetTokenCache() {
	tokenMu.Lock()
	defer tokenMu.Unlock()
	cachedToken = nil
}
