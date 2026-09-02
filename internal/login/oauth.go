package login

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// OAuthConfig holds the OAuth client configuration for ChatGPT login.
// OAuthConfig 保存 ChatGPT 登录的 OAuth 客户端配置。
type OAuthConfig struct {
	// ClientID is the OAuth client identifier.
	// ClientID 是 OAuth 客户端标识符。
	ClientID string
	// RedirectURI is the local callback URL receiving the authorization code.
	// RedirectURI 是接收授权码的本地回调 URL。
	RedirectURI string
	// AuthURL is the authorization endpoint URL.
	// AuthURL 是授权端点 URL。
	AuthURL string
	// TokenURL is the token exchange endpoint URL.
	// TokenURL 是令牌交换端点 URL。
	TokenURL string
}

// DefaultOAuthConfig returns the default OAuth configuration for ChatGPT.
// DefaultOAuthConfig 返回 ChatGPT 的默认 OAuth 配置。
func DefaultOAuthConfig() *OAuthConfig {
	return &OAuthConfig{
		ClientID:    "app_EMoamEEZ73f0CkXaXp7hrann",
		RedirectURI: "http://localhost:1455/auth/callback",
		AuthURL:     "https://auth.openai.com/authorize",
		TokenURL:    "https://auth.openai.com/oauth/token",
	}
}

// StartOAuthFlow initiates the OAuth login flow by starting a local HTTP server
// to receive the callback, then opening the browser for the user to log in.
// StartOAuthFlow 启动 OAuth 登录流程，通过启动本地 HTTP 服务器接收回调，
// 然后打开浏览器供用户登录。
func StartOAuthFlow(ctx context.Context) (*AuthToken, error) {
	cfg := DefaultOAuthConfig()

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	server := &http.Server{Addr: "localhost:1455", Handler: mux}

	mux.HandleFunc("/auth/callback", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		if code == "" {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Missing authorization code"))
			errCh <- fmt.Errorf("missing authorization code")
			return
		}
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<html><body><h1>Login successful!</h1><p>You can close this window.</p></body></html>"))
		codeCh <- code
	})

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	defer server.Shutdown(ctx)

	// Build auth URL.
	// 构建认证 URL。
	authURL := fmt.Sprintf("%s?response_type=code&client_id=%s&redirect_uri=%s&scope=openid+email+profile",
		cfg.AuthURL, cfg.ClientID, url.QueryEscape(cfg.RedirectURI))

	fmt.Printf("Please open this URL in your browser to log in:\n%s\n\n", authURL)
	fmt.Print("Waiting for authentication... ")

	select {
	case code := <-codeCh:
		fmt.Println("Received authorization code.")
		// Exchange code for token.
		// 交换 code 换取 token。
		return exchangeCodeForToken(cfg, code)
	case err := <-errCh:
		return nil, fmt.Errorf("oauth error: %w", err)
	case <-ctx.Done():
		return nil, fmt.Errorf("oauth timed out")
	case <-time.After(5 * time.Minute):
		return nil, fmt.Errorf("oauth timed out after 5 minutes")
	}
}

// exchangeCodeForToken exchanges the authorization code for an access token.
// exchangeCodeForToken 将授权码交换为访问令牌。
func exchangeCodeForToken(cfg *OAuthConfig, code string) (*AuthToken, error) {
	data := url.Values{
		"grant_type":   {"authorization_code"},
		"code":         {code},
		"redirect_uri": {cfg.RedirectURI},
		"client_id":    {cfg.ClientID},
	}

	resp, err := http.PostForm(cfg.TokenURL, data)
	if err != nil {
		return nil, fmt.Errorf("token exchange failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token exchange failed with status: %d", resp.StatusCode)
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		IDToken      string `json:"id_token"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("parse token response: %w", err)
	}

	expiresAt := time.Now()
	if tokenResp.ExpiresIn > 0 {
		expiresAt = expiresAt.Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	}

	return &AuthToken{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresAt:    expiresAt,
	}, nil
}
