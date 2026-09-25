package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	defaultAPIBaseURL   = "https://api.github.com"
	defaultOAuthBaseURL = "https://github.com"
	oauthScope          = "repo,user:email"
	reposPerPage        = 100
	// maxRepoPages bounds the repository listing (10 000 repositories) so a
	// pathological account cannot make one request loop indefinitely.
	maxRepoPages = 100
)

// APIClient is the real Client implementation, talking to GitHub's OAuth
// endpoints and REST API. It is exercised manually/in production; tests use
// a fake Client (network access is not assumed in CI).
type APIClient struct {
	HTTPClient    *http.Client
	BaseURL       string // REST API base, default https://api.github.com
	OAuthBaseURL  string // OAuth base, default https://github.com
	ClientID      string
	ClientSecret  string
	WebhookSecret string
}

var _ Client = (*APIClient)(nil)

func (c *APIClient) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{Timeout: 15 * time.Second}
}

func (c *APIClient) apiBase() string {
	if c.BaseURL != "" {
		return strings.TrimRight(c.BaseURL, "/")
	}
	return defaultAPIBaseURL
}

func (c *APIClient) oauthBase() string {
	if c.OAuthBaseURL != "" {
		return strings.TrimRight(c.OAuthBaseURL, "/")
	}
	return defaultOAuthBaseURL
}

// AuthURL returns the GitHub OAuth authorization URL. The redirect URI is
// the one registered on the OAuth App.
func (c *APIClient) AuthURL(state string) string {
	q := url.Values{}
	q.Set("client_id", c.ClientID)
	q.Set("state", state)
	q.Set("scope", oauthScope)
	return c.oauthBase() + "/login/oauth/authorize?" + q.Encode()
}

// ExchangeCode exchanges an OAuth code for an access token, then fetches
// the authenticated GitHub user.
func (c *APIClient) ExchangeCode(ctx context.Context, code string) (string, int64, string, error) {
	form := url.Values{}
	form.Set("client_id", c.ClientID)
	form.Set("client_secret", c.ClientSecret)
	form.Set("code", code)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.oauthBase()+"/login/oauth/access_token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", 0, "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	var tokenResp struct {
		AccessToken      string `json:"access_token"`
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	if _, err := c.do(req, &tokenResp); err != nil {
		return "", 0, "", err
	}
	if tokenResp.Error != "" || tokenResp.AccessToken == "" {
		// GitHub reports a bad/expired code as 200 with an error field.
		return "", 0, "", fmt.Errorf("%w: github token exchange failed: %s", ErrInvalidOAuthState, tokenResp.Error)
	}

	userReq, err := c.apiRequest(ctx, tokenResp.AccessToken, c.apiBase()+"/user")
	if err != nil {
		return "", 0, "", err
	}
	var ghUser struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
	}
	if _, err := c.do(userReq, &ghUser); err != nil {
		return "", 0, "", err
	}
	if ghUser.ID == 0 || ghUser.Login == "" {
		return "", 0, "", errors.New("github: /user response missing id or login")
	}
	return tokenResp.AccessToken, ghUser.ID, ghUser.Login, nil
}

// ListRepositories returns every repository accessToken can access,
// following GitHub's pagination.
func (c *APIClient) ListRepositories(ctx context.Context, accessToken string) ([]AvailableRepository, error) {
	type apiRepo struct {
		ID            int64  `json:"id"`
		Name          string `json:"name"`
		HTMLURL       string `json:"html_url"`
		Private       bool   `json:"private"`
		DefaultBranch string `json:"default_branch"`
		Owner         struct {
			Login string `json:"login"`
		} `json:"owner"`
	}
	out := make([]AvailableRepository, 0)
	for page := 1; page <= maxRepoPages; page++ {
		endpoint := c.apiBase() + "/user/repos?sort=updated&per_page=" + strconv.Itoa(reposPerPage) + "&page=" + strconv.Itoa(page)
		req, err := c.apiRequest(ctx, accessToken, endpoint)
		if err != nil {
			return nil, err
		}
		var repos []apiRepo
		if _, err := c.do(req, &repos); err != nil {
			return nil, err
		}
		for _, r := range repos {
			out = append(out, AvailableRepository{
				ExternalID: r.ID, OwnerLogin: r.Owner.Login, Name: r.Name,
				HTMLURL: r.HTMLURL, Private: r.Private, DefaultBranch: r.DefaultBranch,
			})
		}
		if len(repos) < reposPerPage {
			break
		}
	}
	return out, nil
}

// VerifyWebhookSignature checks GitHub's X-Hub-Signature-256 header.
func (c *APIClient) VerifyWebhookSignature(payload []byte, signatureHeader string) bool {
	return VerifySignature(c.WebhookSecret, payload, signatureHeader)
}

func (c *APIClient) apiRequest(ctx context.Context, accessToken, endpoint string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	return req, nil
}

// ErrTokenRejected is returned when GitHub rejects the stored access token
// (revoked or expired); the user must reconnect their GitHub account.
var ErrTokenRejected = errors.New("github: access token was rejected by GitHub; reconnect your GitHub account")

func (c *APIClient) do(req *http.Request, dst any) (*http.Response, error) {
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("github: request %s: %w", req.URL.Path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, fmt.Errorf("github: read %s response: %w", req.URL.Path, err)
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, ErrTokenRejected
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("github: %s returned HTTP %d", req.URL.Path, resp.StatusCode)
	}
	if err := json.Unmarshal(body, dst); err != nil {
		return nil, fmt.Errorf("github: decode %s response: %w", req.URL.Path, err)
	}
	return resp, nil
}
