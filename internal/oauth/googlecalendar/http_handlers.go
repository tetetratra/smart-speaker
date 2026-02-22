package googlecalendar

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"strings"
	"time"
)

const (
	oauthStateCookieName = "google_oauth_state"
)

// RegisterHTTPHandlers registers OAuth endpoints on the provided mux.
func RegisterHTTPHandlers(mux *http.ServeMux) {
	if mux == nil {
		return
	}
	mux.HandleFunc("/oauth/google/start", handleOAuthStart)
	mux.HandleFunc("/oauth/google/callback", handleOAuthCallback)
	mux.HandleFunc("/oauth/google/status", handleOAuthStatus)
}

func handleOAuthStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	state, err := RandomState()
	if err != nil {
		renderOAuthResult(w, http.StatusInternalServerError, "Google認証に失敗しました", err.Error(), false)
		return
	}
	authURL, err := AuthURL(state)
	if err != nil {
		renderOAuthResult(w, http.StatusInternalServerError, "Google認証のURL生成に失敗しました", err.Error(), false)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     oauthStateCookieName,
		Value:    state,
		Path:     "/oauth/google",
		MaxAge:   int((10 * time.Minute).Seconds()),
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, authURL, http.StatusFound)
}

func handleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	queryState := strings.TrimSpace(r.URL.Query().Get("state"))
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	if queryState == "" || code == "" {
		renderOAuthResult(w, http.StatusBadRequest, "Google認証に失敗しました", "state または code が不足しています。", false)
		return
	}
	cookie, err := r.Cookie(oauthStateCookieName)
	if err != nil {
		renderOAuthResult(w, http.StatusBadRequest, "Google認証に失敗しました", "state を確認できませんでした。", false)
		return
	}
	if subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(queryState)) != 1 {
		renderOAuthResult(w, http.StatusBadRequest, "Google認証に失敗しました", "state が一致しません。", false)
		return
	}

	cfg, err := LoadConfig()
	if err != nil {
		renderOAuthResult(w, http.StatusInternalServerError, "Google認証に失敗しました", err.Error(), false)
		return
	}
	tok, err := OAuthConfig(cfg).Exchange(r.Context(), code)
	if err != nil {
		renderOAuthResult(w, http.StatusBadRequest, "Google認証に失敗しました", "トークン交換に失敗しました: "+err.Error(), false)
		return
	}
	if err := SaveToken(tok); err != nil {
		renderOAuthResult(w, http.StatusInternalServerError, "Google認証に失敗しました", "トークン保存に失敗しました: "+err.Error(), false)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     oauthStateCookieName,
		Value:    "",
		Path:     "/oauth/google",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
	})
	renderOAuthResult(w, http.StatusOK, "Google認証が完了しました", "このタブは閉じて構いません。", true)
}

func handleOAuthStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	resp := map[string]any{
		"authenticated": false,
	}
	if tok, err := LoadToken(); err == nil && tok != nil {
		resp["authenticated"] = true
		if !tok.Expiry.IsZero() {
			resp["expiry"] = tok.Expiry.Format(time.RFC3339)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func renderOAuthResult(w http.ResponseWriter, status int, title string, message string, success bool) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	safeTitle := html.EscapeString(title)
	safeMessage := html.EscapeString(message)
	color := "#b91c1c"
	if success {
		color = "#166534"
	}
	_, _ = fmt.Fprintf(w, `<!doctype html>
<html lang="ja">
<head><meta charset="utf-8"><title>%s</title></head>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif; margin: 24px;">
  <h1 style="font-size: 20px; color: %s; margin-bottom: 8px;">%s</h1>
  <p style="font-size: 14px; line-height: 1.6;">%s</p>
</body>
</html>`, safeTitle, color, safeTitle, safeMessage)
}
