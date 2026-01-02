package googlecalendar

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

const tokenFile = "tmp/googlecalendar_oauth_token.json"

// LoadToken は保存済みトークンを読み込みます。
func LoadToken() (*oauth2.Token, error) {
	f, err := os.Open(tokenFile)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var tok oauth2.Token
	if err := json.NewDecoder(f).Decode(&tok); err != nil {
		return nil, err
	}
	return &tok, nil
}

// SaveToken はトークンを保存します。
func SaveToken(tok *oauth2.Token) error {
	if tok == nil {
		return fmt.Errorf("token is nil")
	}
	if err := os.MkdirAll(filepath.Dir(tokenFile), 0o755); err != nil {
		return err
	}
	f, err := os.Create(tokenFile)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(tok)
}

// AccessToken は保存済みトークンを読み込み、必要なら更新して返します。
func AccessToken(ctx context.Context) (string, error) {
	cfg, err := LoadConfig()
	if err != nil {
		return "", err
	}
	tok, err := LoadToken()
	if err != nil {
		return "", err
	}
	src := OAuthConfig(cfg).TokenSource(ctx, tok)
	refreshed, err := src.Token()
	if err != nil {
		return "", err
	}
	if refreshed.AccessToken != tok.AccessToken || refreshed.Expiry.After(tok.Expiry.Add(1*time.Minute)) {
		if err := SaveToken(refreshed); err != nil {
			return "", err
		}
	}
	return strings.TrimSpace(refreshed.AccessToken), nil
}
