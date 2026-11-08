package googlecalendar

import (
	"crypto/rand"
	"encoding/hex"

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

// RandomState は簡易なstate文字列を生成します。
func RandomState() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
