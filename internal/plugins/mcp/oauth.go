package mcp

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"golang.org/x/oauth2"
)

// authRequiredError は、認可されていないか、認可が切れているときのエラー。会話モデルにもそのまま返す。
type authRequiredError struct {
	server string
	cause  error
}

func (e *authRequiredError) Error() string {
	msg := fmt.Sprintf("MCPサーバー %s は未認可か、認可が切れています。ブラウザで /oauth/mcp/start?server=%s を開いて認可し、サーバーを再起動してください", e.server, e.server)
	if e.cause != nil {
		msg += fmt.Sprintf("（%v）", e.cause)
	}
	return msg
}

func (e *authRequiredError) Unwrap() error { return e.cause }

// authMetadata は、保護リソースと認可サーバーのメタデータから得たもの。
type authMetadata struct {
	// resource は保護リソースの識別子。認可とトークンのリクエストに resource として付ける。
	resource              string
	scopesSupported       []string
	authorizationEndpoint string
	tokenEndpoint         string
	registrationEndpoint  string
}

// oauthServer は、OAuth で認可する MCP サーバー1つ分の状態。
type oauthServer struct {
	name string
	url  string
	cfg  OAuthConfig

	mu sync.Mutex
	// meta はメタデータ。最初に要るときに取る。
	meta *authMetadata
	// tokens は保存したトークンからの TokenSource。期限が切れたら更新トークンで更新し、保存し直す。
	tokens oauth2.TokenSource
}

type pendingAuth struct {
	server   *oauthServer
	config   *oauth2.Config
	verifier string
}

// oauthManager は、MCP サーバーの OAuth（認可の開始・コールバック・トークンの更新）をまとめて扱う。
type oauthManager struct {
	redirectURL string
	store       *tokenStore
	httpClient  *http.Client

	mu      sync.Mutex
	servers map[string]*oauthServer
	pending map[string]pendingAuth
}

func newOAuthManager(redirectURL string, store *tokenStore, httpClient *http.Client) *oauthManager {
	return &oauthManager{
		redirectURL: redirectURL,
		store:       store,
		httpClient:  httpClient,
		servers:     map[string]*oauthServer{},
		pending:     map[string]pendingAuth{},
	}
}

func (m *oauthManager) add(serverName, serverURL string, cfg OAuthConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.servers[serverName] = &oauthServer{name: serverName, url: serverURL, cfg: cfg}
}

func (m *oauthManager) server(serverName string) *oauthServer {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.servers[serverName]
}

// token は、サーバーへ付けるアクセストークンを返す。保存したトークンがない・更新できないときは authRequiredError。
func (m *oauthManager) token(ctx context.Context, serverName string) (*oauth2.Token, error) {
	s := m.server(serverName)
	if s == nil {
		return nil, fmt.Errorf("MCPサーバー %s の OAuth の設定がありません", serverName)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tokens == nil {
		saved, err := m.store.load(serverName)
		if err != nil {
			return nil, err
		}
		if saved.Token == nil {
			return nil, &authRequiredError{server: serverName}
		}
		meta, err := m.metadata(ctx, s)
		if err != nil {
			return nil, err
		}
		config := m.oauthConfig(s, meta, saved)
		refreshCtx := context.WithValue(context.Background(), oauth2.HTTPClient, m.httpClient)
		s.tokens = &savingTokenSource{base: config.TokenSource(refreshCtx, saved.Token), store: m.store, server: serverName, last: saved.Token.AccessToken}
	}
	tok, err := s.tokens.Token()
	if err != nil {
		return nil, &authRequiredError{server: serverName, cause: err}
	}
	return tok, nil
}

// metadata はメタデータを返す。s.mu を持って呼ぶ。
func (m *oauthManager) metadata(ctx context.Context, s *oauthServer) (*authMetadata, error) {
	if s.meta != nil {
		return s.meta, nil
	}
	meta, err := discover(ctx, m.httpClient, s.url)
	if err != nil {
		return nil, fmt.Errorf("MCPサーバー %s の認可サーバーを見つけられません: %w", s.name, err)
	}
	s.meta = meta
	return meta, nil
}

// oauthConfig は、設定の OAuth クライアントか、動的クライアント登録で得たクライアントで oauth2.Config を作る。
func (m *oauthManager) oauthConfig(s *oauthServer, meta *authMetadata, saved savedServer) *oauth2.Config {
	clientID, clientSecret := s.cfg.ClientID, s.cfg.ClientSecret
	if clientID == "" {
		clientID, clientSecret = saved.ClientID, saved.ClientSecret
	}
	scopes := s.cfg.Scopes
	if len(scopes) == 0 {
		scopes = meta.scopesSupported
	}
	return &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint:     oauth2.Endpoint{AuthURL: meta.authorizationEndpoint, TokenURL: meta.tokenEndpoint},
		RedirectURL:  m.redirectURL,
		Scopes:       scopes,
	}
}

func (m *oauthManager) registerHTTP(mux *http.ServeMux) {
	mux.HandleFunc("GET /oauth/mcp/start", m.handleStart)
	mux.HandleFunc("GET /oauth/mcp/callback", m.handleCallback)
}

// handleStart は認可を始める。必要なら動的クライアント登録をしてから、認可サーバーへリダイレクトする。
func (m *oauthManager) handleStart(w http.ResponseWriter, r *http.Request) {
	serverName := r.URL.Query().Get("server")
	s := m.server(serverName)
	if s == nil {
		http.Error(w, "oauth が設定された MCP サーバーではありません: "+serverName, http.StatusNotFound)
		return
	}
	ctx := r.Context()
	s.mu.Lock()
	meta, err := m.metadata(ctx, s)
	s.mu.Unlock()
	if err != nil {
		log.Printf("mcp: %s の認可を始められません: %v", serverName, err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	saved, err := m.store.load(serverName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if s.cfg.ClientID == "" && saved.ClientID == "" {
		saved.ClientID, saved.ClientSecret, err = registerClient(ctx, m.httpClient, meta.registrationEndpoint, m.redirectURL)
		if err != nil {
			log.Printf("mcp: %s の動的クライアント登録に失敗しました: %v", serverName, err)
			http.Error(w, fmt.Sprintf("動的クライアント登録に失敗しました。対応していないサーバーなら、plugins.mcp.servers.%s.oauth に client_id と client_secret を書いてください: %v", serverName, err), http.StatusBadGateway)
			return
		}
		if err := m.store.saveClient(serverName, saved.ClientID, saved.ClientSecret); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	config := m.oauthConfig(s, meta, saved)
	state, err := randomString()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	verifier := oauth2.GenerateVerifier()
	m.mu.Lock()
	m.pending[state] = pendingAuth{server: s, config: config, verifier: verifier}
	m.mu.Unlock()
	authURL := config.AuthCodeURL(state,
		oauth2.S256ChallengeOption(verifier),
		oauth2.AccessTypeOffline,
		oauth2.SetAuthURLParam("prompt", "consent"),
		oauth2.SetAuthURLParam("resource", meta.resource),
	)
	http.Redirect(w, r, authURL, http.StatusFound)
}

// handleCallback は認可コードをトークンに換えて保存する。
func (m *oauthManager) handleCallback(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	state := query.Get("state")
	m.mu.Lock()
	pending, ok := m.pending[state]
	delete(m.pending, state)
	m.mu.Unlock()
	if !ok {
		http.Error(w, "認可の state が一致しません。/oauth/mcp/start からやり直してください", http.StatusBadRequest)
		return
	}
	s := pending.server
	if errCode := query.Get("error"); errCode != "" {
		log.Printf("mcp: %s の認可が拒否されました: %s", s.name, errCode)
		http.Error(w, "認可されませんでした: "+errCode, http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	resource := ""
	if s.meta != nil {
		resource = s.meta.resource
	}
	s.mu.Unlock()
	ctx := context.WithValue(r.Context(), oauth2.HTTPClient, m.httpClient)
	tok, err := pending.config.Exchange(ctx, query.Get("code"),
		oauth2.VerifierOption(pending.verifier),
		oauth2.SetAuthURLParam("resource", resource),
	)
	if err != nil {
		log.Printf("mcp: %s のトークンを取れませんでした: %v", s.name, err)
		http.Error(w, "トークンを取れませんでした", http.StatusBadGateway)
		return
	}
	if err := m.store.saveToken(s.name, tok); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.mu.Lock()
	s.tokens = nil // 次に使うとき、保存したトークンから作り直す。
	s.mu.Unlock()
	log.Printf("mcp: %s を認可しました", s.name)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, "<!doctype html><meta charset=utf-8><p>MCPサーバー %s を認可しました。サーバーを再起動すると、ツールを使えるようになります。</p>", html.EscapeString(s.name))
}

// oauthTransport は、リクエストにアクセストークンを付ける。
type oauthTransport struct {
	base   http.RoundTripper
	oauth  *oauthManager
	server string
}

func (t *oauthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	tok, err := t.oauth.token(req.Context(), t.server)
	if err != nil {
		return nil, err
	}
	req = req.Clone(req.Context())
	tok.SetAuthHeader(req)
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusUnauthorized {
		_ = resp.Body.Close()
		return nil, &authRequiredError{server: t.server, cause: errors.New("サーバーが 401 を返しました")}
	}
	return resp, nil
}

// savingTokenSource は、トークンが更新されたら保存し直す。
type savingTokenSource struct {
	base   oauth2.TokenSource
	store  *tokenStore
	server string

	mu   sync.Mutex
	last string
}

func (s *savingTokenSource) Token() (*oauth2.Token, error) {
	tok, err := s.base.Token()
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if tok.AccessToken != s.last {
		if err := s.store.saveToken(s.server, tok); err != nil {
			log.Printf("mcp: %s の更新したトークンを保存できませんでした: %v", s.server, err)
		}
		s.last = tok.AccessToken
	}
	return tok, nil
}

// discover は、MCP サーバーの URL から保護リソースのメタデータ（RFC 9728）と認可サーバーのメタデータ（RFC 8414・OpenID Connect Discovery）を取る。
func discover(ctx context.Context, client *http.Client, serverURL string) (*authMetadata, error) {
	u, err := url.Parse(serverURL)
	if err != nil {
		return nil, err
	}
	origin := u.Scheme + "://" + u.Host
	path := strings.TrimSuffix(u.Path, "/")

	var candidates []string
	if metadataURL := resourceMetadataFromChallenge(ctx, client, serverURL); metadataURL != "" {
		candidates = append(candidates, metadataURL)
	}
	if path != "" {
		candidates = append(candidates, origin+"/.well-known/oauth-protected-resource"+path)
	}
	candidates = append(candidates, origin+"/.well-known/oauth-protected-resource")
	var resource struct {
		Resource             string   `json:"resource"`
		AuthorizationServers []string `json:"authorization_servers"`
		ScopesSupported      []string `json:"scopes_supported"`
	}
	if err := getFirstJSON(ctx, client, candidates, &resource); err != nil {
		return nil, fmt.Errorf("保護リソースのメタデータ: %w", err)
	}
	if len(resource.AuthorizationServers) == 0 {
		return nil, errors.New("保護リソースのメタデータに authorization_servers がありません")
	}

	issuer, err := url.Parse(resource.AuthorizationServers[0])
	if err != nil {
		return nil, err
	}
	issuerOrigin := issuer.Scheme + "://" + issuer.Host
	issuerPath := strings.TrimSuffix(issuer.Path, "/")
	var server struct {
		AuthorizationEndpoint string `json:"authorization_endpoint"`
		TokenEndpoint         string `json:"token_endpoint"`
		RegistrationEndpoint  string `json:"registration_endpoint"`
	}
	asCandidates := []string{
		issuerOrigin + "/.well-known/oauth-authorization-server" + issuerPath,
		issuerOrigin + "/.well-known/openid-configuration" + issuerPath,
	}
	if issuerPath != "" {
		asCandidates = append(asCandidates, issuerOrigin+issuerPath+"/.well-known/openid-configuration")
	}
	if err := getFirstJSON(ctx, client, asCandidates, &server); err != nil {
		return nil, fmt.Errorf("認可サーバーのメタデータ: %w", err)
	}
	if server.AuthorizationEndpoint == "" || server.TokenEndpoint == "" {
		return nil, errors.New("認可サーバーのメタデータに authorization_endpoint か token_endpoint がありません")
	}
	resourceID := resource.Resource
	if resourceID == "" {
		resourceID = serverURL
	}
	return &authMetadata{
		resource:              resourceID,
		scopesSupported:       resource.ScopesSupported,
		authorizationEndpoint: server.AuthorizationEndpoint,
		tokenEndpoint:         server.TokenEndpoint,
		registrationEndpoint:  server.RegistrationEndpoint,
	}, nil
}

// resourceMetadataFromChallenge は、認可なしで MCP サーバーを叩き、401 の WWW-Authenticate にある resource_metadata の URL を返す。なければ空。
func resourceMetadataFromChallenge(ctx context.Context, client *http.Client, serverURL string) string {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, serverURL, strings.NewReader(`{"jsonrpc":"2.0","id":0,"method":"ping"}`))
	if err != nil {
		return ""
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		return ""
	}
	for _, challenge := range resp.Header.Values("WWW-Authenticate") {
		_, params, err := mime.ParseMediaType(strings.Replace(challenge, " ", "; ", 1))
		if err == nil && params["resource_metadata"] != "" {
			return params["resource_metadata"]
		}
	}
	return ""
}

// getFirstJSON は、candidates を順に GET し、最初に 200 を返したものの JSON を v に読む。
func getFirstJSON(ctx context.Context, client *http.Client, candidates []string, v any) error {
	var lastErr error
	for _, candidate := range candidates {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, candidate, nil)
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("Accept", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		_ = resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("%s: HTTP %d", candidate, resp.StatusCode)
			continue
		}
		if err := json.Unmarshal(body, v); err != nil {
			lastErr = fmt.Errorf("%s: %w", candidate, err)
			continue
		}
		return nil
	}
	return lastErr
}

// registerClient は動的クライアント登録（RFC 7591）をして、クライアントの ID とシークレットを返す。
func registerClient(ctx context.Context, client *http.Client, endpoint, redirectURL string) (string, string, error) {
	if endpoint == "" {
		return "", "", errors.New("認可サーバーが動的クライアント登録に対応していません")
	}
	payload, err := json.Marshal(map[string]any{
		"client_name":                "smart-speaker",
		"redirect_uris":              []string{redirectURL},
		"grant_types":                []string{"authorization_code", "refresh_token"},
		"response_types":             []string{"code"},
		"token_endpoint_auth_method": "none",
	})
	if err != nil {
		return "", "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", "", err
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var registered struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
	}
	if err := json.Unmarshal(body, &registered); err != nil {
		return "", "", err
	}
	if registered.ClientID == "" {
		return "", "", errors.New("client_id が返りませんでした")
	}
	return registered.ClientID, registered.ClientSecret, nil
}

func randomString() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
