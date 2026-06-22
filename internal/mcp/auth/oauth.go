package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
	"time"
)

type PKCEFlow struct {
	ServerID    string
	ResourceURL string
	Store       TokenStore
	HTTPClient  *http.Client
	OpenBrowser func(string) error
	Now         func() time.Time
}

type tokenEnvelope struct {
	AccessToken  string    `json:"access_token"`
	TokenType    string    `json:"token_type,omitempty"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	Expiry       time.Time `json:"expiry,omitempty"`
}

type resourceMetadata struct {
	AuthorizationServers []string `json:"authorization_servers"`
}

type authServerMetadata struct {
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
}

func (f *PKCEFlow) EnsureToken(ctx context.Context) (string, error) {
	if strings.TrimSpace(f.ServerID) == "" {
		return "", fmt.Errorf("server id is required")
	}
	if strings.TrimSpace(f.ResourceURL) == "" {
		return "", fmt.Errorf("resource url is required")
	}
	if f.Store == nil {
		return "", fmt.Errorf("token store is required")
	}
	if tok, err := f.loadToken(); err == nil {
		if validAccessToken(tok, f.now()) {
			return tok.AccessToken, nil
		}
	}

	rm, err := f.fetchResourceMetadata(ctx)
	if err != nil {
		return "", err
	}
	if len(rm.AuthorizationServers) == 0 {
		return "", fmt.Errorf("no authorization servers declared")
	}
	am, err := f.fetchAuthServerMetadata(ctx, rm.AuthorizationServers[0])
	if err != nil {
		return "", err
	}

	if tok, err := f.loadToken(); err == nil && strings.TrimSpace(tok.RefreshToken) != "" {
		refreshed, err := f.refreshToken(ctx, am.TokenEndpoint, tok.RefreshToken)
		if err == nil && validAccessToken(refreshed, f.now()) {
			if saveErr := f.saveToken(refreshed); saveErr != nil {
				return "", saveErr
			}
			return refreshed.AccessToken, nil
		}
	}

	verifier, challenge, err := generatePKCE()
	if err != nil {
		return "", err
	}
	state, err := randomBase64URL(24)
	if err != nil {
		return "", err
	}
	redirectURI, codeCh, stop, err := startCallbackServer(ctx)
	if err != nil {
		return "", err
	}
	defer stop()

	authURL, err := buildAuthURL(am.AuthorizationEndpoint, map[string]string{
		"response_type":         "code",
		"client_id":             "nandocodego-mcp-public",
		"redirect_uri":          redirectURI,
		"scope":                 "mcp",
		"state":                 state,
		"code_challenge":        challenge,
		"code_challenge_method": "S256",
	})
	if err != nil {
		return "", err
	}
	if err := f.browserOpen(authURL); err != nil {
		return "", err
	}

	var code string
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case got := <-codeCh:
		code = got
	}
	if strings.TrimSpace(code) == "" {
		return "", fmt.Errorf("missing authorization code")
	}
	tok, err := f.exchangeCode(ctx, am.TokenEndpoint, code, verifier, redirectURI)
	if err != nil {
		return "", err
	}
	if err := f.saveToken(tok); err != nil {
		return "", err
	}
	return tok.AccessToken, nil
}

func (f *PKCEFlow) fetchResourceMetadata(ctx context.Context) (resourceMetadata, error) {
	u := strings.TrimRight(f.ResourceURL, "/") + "/.well-known/oauth-protected-resource"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return resourceMetadata{}, err
	}
	resp, err := f.client().Do(req)
	if err != nil {
		return resourceMetadata{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resourceMetadata{}, fmt.Errorf("resource metadata status %d", resp.StatusCode)
	}
	var out resourceMetadata
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return resourceMetadata{}, err
	}
	return out, nil
}

func (f *PKCEFlow) fetchAuthServerMetadata(ctx context.Context, issuer string) (authServerMetadata, error) {
	u := strings.TrimRight(issuer, "/") + "/.well-known/oauth-authorization-server"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return authServerMetadata{}, err
	}
	resp, err := f.client().Do(req)
	if err != nil {
		return authServerMetadata{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return authServerMetadata{}, fmt.Errorf("auth metadata status %d", resp.StatusCode)
	}
	var out authServerMetadata
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return authServerMetadata{}, err
	}
	return out, nil
}

func (f *PKCEFlow) refreshToken(ctx context.Context, tokenEndpoint, refreshToken string) (tokenEnvelope, error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	form.Set("client_id", "nandocodego-mcp-public")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return tokenEnvelope{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := f.client().Do(req)
	if err != nil {
		return tokenEnvelope{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return tokenEnvelope{}, fmt.Errorf("token refresh failed: status=%d body=%s", resp.StatusCode, string(b))
	}
	return decodeTokenResponse(resp.Body)
}

func (f *PKCEFlow) exchangeCode(ctx context.Context, tokenEndpoint, code, verifier, redirectURI string) (tokenEnvelope, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("client_id", "nandocodego-mcp-public")
	form.Set("redirect_uri", redirectURI)
	form.Set("code_verifier", verifier)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return tokenEnvelope{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := f.client().Do(req)
	if err != nil {
		return tokenEnvelope{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return tokenEnvelope{}, fmt.Errorf("token exchange failed: status=%d body=%s", resp.StatusCode, string(b))
	}
	return decodeTokenResponse(resp.Body)
}

func decodeTokenResponse(r io.Reader) (tokenEnvelope, error) {
	var raw struct {
		AccessToken  string `json:"access_token"`
		TokenType    string `json:"token_type"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	if err := json.NewDecoder(r).Decode(&raw); err != nil {
		return tokenEnvelope{}, err
	}
	tok := tokenEnvelope{
		AccessToken:  raw.AccessToken,
		TokenType:    raw.TokenType,
		RefreshToken: raw.RefreshToken,
	}
	if raw.ExpiresIn > 0 {
		tok.Expiry = time.Now().UTC().Add(time.Duration(raw.ExpiresIn) * time.Second)
	}
	if strings.TrimSpace(tok.AccessToken) == "" {
		return tokenEnvelope{}, fmt.Errorf("missing access_token")
	}
	return tok, nil
}

func (f *PKCEFlow) loadToken() (tokenEnvelope, error) {
	raw, err := f.Store.Load(f.ServerID)
	if err != nil {
		return tokenEnvelope{}, err
	}
	var tok tokenEnvelope
	if err := json.Unmarshal([]byte(raw), &tok); err != nil {
		return tokenEnvelope{}, err
	}
	return tok, nil
}

func (f *PKCEFlow) saveToken(tok tokenEnvelope) error {
	b, err := json.Marshal(tok)
	if err != nil {
		return err
	}
	return f.Store.Save(f.ServerID, string(b))
}

func validAccessToken(tok tokenEnvelope, now time.Time) bool {
	if strings.TrimSpace(tok.AccessToken) == "" {
		return false
	}
	if tok.Expiry.IsZero() {
		return true
	}
	return tok.Expiry.After(now.Add(30 * time.Second))
}

func generatePKCE() (string, string, error) {
	verifier, err := randomBase64URL(32)
	if err != nil {
		return "", "", err
	}
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge, nil
}

func randomBase64URL(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func buildAuthURL(endpoint string, q map[string]string) (string, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}
	values := u.Query()
	for k, v := range q {
		values.Set(k, v)
	}
	u.RawQuery = values.Encode()
	return u.String(), nil
}

func startCallbackServer(ctx context.Context) (redirectURI string, codeCh <-chan string, stop func(), err error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, nil, err
	}
	ch := make(chan string, 1)
	mux := http.NewServeMux()
	server := &http.Server{Handler: mux}
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		select {
		case ch <- code:
		default:
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Authentication complete. You can close this window."))
		go server.Shutdown(context.Background())
	})
	go server.Serve(ln)
	go func() {
		<-ctx.Done()
		_ = server.Shutdown(context.Background())
	}()
	stopFn := func() {
		_ = server.Shutdown(context.Background())
	}
	return "http://" + ln.Addr().String() + "/callback", ch, stopFn, nil
}

func (f *PKCEFlow) client() *http.Client {
	if f.HTTPClient != nil {
		return f.HTTPClient
	}
	return &http.Client{Timeout: 30 * time.Second}
}

func (f *PKCEFlow) now() time.Time {
	if f.Now != nil {
		return f.Now()
	}
	return time.Now().UTC()
}

func (f *PKCEFlow) browserOpen(url string) error {
	if f.OpenBrowser != nil {
		return f.OpenBrowser(url)
	}
	return openInBrowser(url)
}

func openInBrowser(rawURL string) error {
	cmd := exec.Command("open", rawURL)
	if err := cmd.Run(); err == nil {
		return nil
	}
	cmd = exec.Command("xdg-open", rawURL)
	if err := cmd.Run(); err == nil {
		return nil
	}
	return fmt.Errorf("failed to open browser")
}
