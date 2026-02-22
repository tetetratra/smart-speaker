package googlecalendar

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"
)

var (
	tokenMu     sync.RWMutex
	cachedToken *oauth2.Token
	errNoToken  = errors.New("google oauth token is not authenticated")
)

// LoadToken はメモリ上のトークンを読み込みます。
func LoadToken() (*oauth2.Token, error) {
	tokenMu.RLock()
	defer tokenMu.RUnlock()
	if cachedToken == nil {
		return nil, errNoToken
	}
	cloned := *cachedToken
	return &cloned, nil
}

// SaveToken はトークンをメモリに保存します。
func SaveToken(tok *oauth2.Token) error {
	if tok == nil {
		return fmt.Errorf("token is nil")
	}
	cloned := *tok
	tokenMu.Lock()
	cachedToken = &cloned
	tokenMu.Unlock()
	return nil
}

// AccessToken はメモリ上のトークンを読み込み、必要なら更新して返します。
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
