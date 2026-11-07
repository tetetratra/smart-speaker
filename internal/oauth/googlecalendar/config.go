package googlecalendar

import (
	"fmt"
	"strings"
	"sync"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const (
	// DefaultRedirectURL は設定ファイルで指定しない場合のOAuthコールバックURLです。
	DefaultRedirectURL = "http://localhost:8081/oauth/google/callback"
	// DefaultScope は設定ファイルで指定しない場合のOAuthスコープです。
	DefaultScope       = scopeCalendarWrite
	scopeCalendarWrite = "https://www.googleapis.com/auth/calendar.events"
)

// Config はGoogle OAuthの設定を持ちます。
type Config struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
	Scopes       []string
	// TokenPath はトークンの保存先です。空の場合は data/google_oauth_token.json を使います。
	TokenPath string
}

var (
	configMu sync.RWMutex
	current  = Config{RedirectURL: DefaultRedirectURL, Scopes: []string{DefaultScope}}
)

// Configure は起動時に読み込んだOAuth設定を登録します。
func Configure(cfg Config) {
	configMu.Lock()
	defer configMu.Unlock()
	current = cfg
}

func currentConfig() Config {
	configMu.RLock()
	defer configMu.RUnlock()
	return current
}

// LoadConfig は登録済みのOAuth設定を返します。client id/secret が無い場合はエラーにします。
func LoadConfig() (Config, error) {
	cfg := currentConfig()
	if strings.TrimSpace(cfg.ClientID) == "" || strings.TrimSpace(cfg.ClientSecret) == "" {
		return Config{}, fmt.Errorf("google oauth client id/secret is required")
	}
	return cfg, nil
}

// OAuthConfig はoauth2.Configを生成します。
func OAuthConfig(cfg Config) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURL,
		Scopes:       cfg.Scopes,
		Endpoint:     google.Endpoint,
	}
}
