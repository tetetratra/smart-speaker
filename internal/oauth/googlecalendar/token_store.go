package googlecalendar

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/oauth2"
)

const defaultTokenPath = "data/google_oauth_token.json"

var (
	tokenMu     sync.RWMutex
	cachedToken *oauth2.Token
	errNoToken  = errors.New("google oauth token is not authenticated")
)

// LoadToken はメモリまたは永続ファイルからトークンを読み込みます。
func LoadToken() (*oauth2.Token, error) {
	tokenMu.RLock()
	if cachedToken != nil {
		tok := cloneToken(cachedToken)
		tokenMu.RUnlock()
		return tok, nil
	}
	tokenMu.RUnlock()

	tokenMu.Lock()
	defer tokenMu.Unlock()
	if cachedToken != nil {
		return cloneToken(cachedToken), nil
	}
	tok, err := loadTokenFromDisk(tokenFilePath())
	if err != nil {
		return nil, err
	}
	cachedToken = cloneToken(tok)
	return cloneToken(cachedToken), nil
}

// SaveToken はトークンをメモリと永続ファイルへ保存します。
func SaveToken(tok *oauth2.Token) error {
	if tok == nil {
		return fmt.Errorf("token is nil")
	}

	tokenMu.Lock()
	defer tokenMu.Unlock()

	existing, _ := loadCachedOrDiskTokenLocked()
	normalized := mergeToken(tok, existing)
	if err := saveTokenToDisk(tokenFilePath(), normalized); err != nil {
		return err
	}
	cachedToken = cloneToken(normalized)
	return nil
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
	merged := mergeToken(refreshed, tok)
	if tokenChanged(tok, merged) {
		if err := SaveToken(merged); err != nil {
			return "", err
		}
	}
	return strings.TrimSpace(merged.AccessToken), nil
}

func tokenFilePath() string {
	if path := strings.TrimSpace(os.Getenv("GOOGLE_OAUTH_TOKEN_PATH")); path != "" {
		return path
	}
	return defaultTokenPath
}

func loadTokenFromDisk(path string) (*oauth2.Token, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, errNoToken
		}
		return nil, err
	}
	var tok oauth2.Token
	if err := json.Unmarshal(raw, &tok); err != nil {
		return nil, err
	}
	if strings.TrimSpace(tok.AccessToken) == "" && strings.TrimSpace(tok.RefreshToken) == "" {
		return nil, errNoToken
	}
	return &tok, nil
}

func saveTokenToDisk(path string, tok *oauth2.Token) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	raw, err := json.Marshal(tok)
	if err != nil {
		return err
	}
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, raw, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

func loadCachedOrDiskTokenLocked() (*oauth2.Token, error) {
	if cachedToken != nil {
		return cloneToken(cachedToken), nil
	}
	tok, err := loadTokenFromDisk(tokenFilePath())
	if err != nil {
		return nil, err
	}
	return tok, nil
}

func mergeToken(current, existing *oauth2.Token) *oauth2.Token {
	merged := cloneToken(current)
	if merged == nil {
		return nil
	}
	if strings.TrimSpace(merged.RefreshToken) == "" && existing != nil {
		merged.RefreshToken = strings.TrimSpace(existing.RefreshToken)
	}
	return merged
}

func tokenChanged(before, after *oauth2.Token) bool {
	if before == nil || after == nil {
		return before != after
	}
	return before.AccessToken != after.AccessToken ||
		before.RefreshToken != after.RefreshToken ||
		before.TokenType != after.TokenType ||
		!before.Expiry.Equal(after.Expiry)
}

func cloneToken(tok *oauth2.Token) *oauth2.Token {
	if tok == nil {
		return nil
	}
	cloned := *tok
	return &cloned
}
