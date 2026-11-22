package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/oauth2"
)

// savedServer は、MCP サーバー1つ分の保存した認可情報。
type savedServer struct {
	// ClientID と ClientSecret は、動的クライアント登録で得たクライアント。設定に書いたクライアントは保存しない。
	ClientID     string        `json:"client_id,omitempty"`
	ClientSecret string        `json:"client_secret,omitempty"`
	Token        *oauth2.Token `json:"token,omitempty"`
}

// tokenStore は、MCP サーバーごとの認可情報を1つの JSON ファイルに保存する。
type tokenStore struct {
	path string
	mu   sync.Mutex
}

func (s *tokenStore) load(serverName string) (savedServer, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	all, err := s.readAll()
	if err != nil {
		return savedServer{}, err
	}
	return all[serverName], nil
}

func (s *tokenStore) saveToken(serverName string, tok *oauth2.Token) error {
	return s.update(serverName, func(saved *savedServer) { saved.Token = tok })
}

func (s *tokenStore) saveClient(serverName, clientID, clientSecret string) error {
	return s.update(serverName, func(saved *savedServer) {
		saved.ClientID, saved.ClientSecret = clientID, clientSecret
	})
}

func (s *tokenStore) update(serverName string, change func(*savedServer)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	all, err := s.readAll()
	if err != nil {
		return err
	}
	saved := all[serverName]
	change(&saved)
	all[serverName] = saved
	data, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("mcp: トークンの保存先を作れません: %w", err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("mcp: トークンを保存できません: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("mcp: トークンを保存できません: %w", err)
	}
	return nil
}

func (s *tokenStore) readAll() (map[string]savedServer, error) {
	all := map[string]savedServer{}
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return all, nil
	}
	if err != nil {
		return nil, fmt.Errorf("mcp: トークンを読めません: %w", err)
	}
	if err := json.Unmarshal(data, &all); err != nil {
		return nil, fmt.Errorf("mcp: トークンのファイルが壊れています: %w", err)
	}
	return all, nil
}
