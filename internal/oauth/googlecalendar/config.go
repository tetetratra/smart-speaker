package googlecalendar

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const (
	defaultRedirectURL = "http://localhost:8081/oauth/google/callback"
	scopeCalendarRead  = "https://www.googleapis.com/auth/calendar.readonly"
	scopeCalendarWrite = "https://www.googleapis.com/auth/calendar.events"
)

// Config はGoogle OAuthの設定を持ちます。
type Config struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
	Scopes       []string
}

// LoadConfig は環境変数からOAuth設定を読み込みます。
func LoadConfig() (Config, error) {
	clientID := strings.TrimSpace(os.Getenv("GOOGLE_CLIENT_ID"))
	clientSecret := strings.TrimSpace(os.Getenv("GOOGLE_CLIENT_SECRET"))
	if clientID == "" || clientSecret == "" {
		return Config{}, fmt.Errorf("google oauth client id/secret is required")
	}
	redirectURL := strings.TrimSpace(os.Getenv("GOOGLE_REDIRECT_URL"))
	if redirectURL == "" {
		redirectURL = defaultRedirectURL
	}
	scope := strings.TrimSpace(os.Getenv("GOOGLE_OAUTH_SCOPE"))
	if scope == "" {
		scope = scopeCalendarWrite
	}
	return Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
		Scopes:       []string{scope},
	}, nil
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

// Scopes は使えるスコープ一覧を返します。
func Scopes() []string {
	return []string{scopeCalendarRead, scopeCalendarWrite}
}
