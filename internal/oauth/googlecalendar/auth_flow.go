package googlecalendar

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"os/exec"
	"runtime"
	"sync"
	"time"

	"golang.org/x/oauth2"
)

// AuthURL は認可URLを生成します。
func AuthURL(state string) (string, error) {
	cfg, err := LoadConfig()
	if err != nil {
		return "", err
	}
	oauthCfg := OAuthConfig(cfg)
	return oauthCfg.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce), nil
}

// ServeAuthCallback はOAuthコールバックを受け取り、トークンを保存します。
func ServeAuthCallback(addr string, stopAfter time.Duration) error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	oauthCfg := OAuthConfig(cfg)
	mux := http.NewServeMux()
	server := &http.Server{Addr: addr, Handler: mux}
	var once sync.Once
	closeServer := func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
		defer cancel()
		_ = server.Shutdown(ctx)
	}
	mux.HandleFunc("/google/callback", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		if code == "" {
			http.Error(w, "missing code", http.StatusBadRequest)
			return
		}
		tok, err := oauthCfg.Exchange(r.Context(), code)
		if err != nil {
			http.Error(w, "exchange failed: "+err.Error(), http.StatusBadRequest)
			return
		}
		if err := SaveToken(tok); err != nil {
			http.Error(w, "save failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		fmt.Fprintln(w, "認証が完了しました。ウィンドウを閉じてください。")
		once.Do(closeServer)
	})
	if stopAfter > 0 {
		go func() {
			time.Sleep(stopAfter)
			once.Do(closeServer)
		}()
	}
	return server.ListenAndServe()
}

// StartAuthFlow は認可URLを開き、コールバック受信を開始します。
func StartAuthFlow(addr string) error {
	state, err := RandomState()
	if err != nil {
		return err
	}
	authURL, err := AuthURL(state)
	if err != nil {
		return err
	}
	if err := openBrowser(authURL); err != nil {
		return err
	}
	return ServeAuthCallback(addr, time.Minute*10)
}

// RandomState は簡易なstate文字列を生成します。
func RandomState() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}
