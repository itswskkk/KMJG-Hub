package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
)

func TestAPIClientAgainstFakeGitHub(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /login/oauth/access_token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("client_id") != "cid" || r.Form.Get("client_secret") != "csecret" {
			t.Errorf("bad client credentials: %v", r.Form)
		}
		if r.Form.Get("code") != "good" {
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "bad_verification_code"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "tok"})
	})
	mux.HandleFunc("GET /user", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 99, "login": "octo"})
	})
	mux.HandleFunc("GET /user/repos", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		n := 0
		switch page {
		case 1:
			n = reposPerPage
		case 2:
			n = 3
		}
		repos := make([]map[string]any, n)
		for i := range repos {
			id := (page-1)*reposPerPage + i + 1
			repos[i] = map[string]any{"id": id, "name": fmt.Sprintf("r%d", id), "html_url": "https://x/" + strconv.Itoa(id),
				"private": true, "default_branch": "main", "owner": map[string]string{"login": "octo"}}
		}
		_ = json.NewEncoder(w).Encode(repos)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := &APIClient{BaseURL: srv.URL, OAuthBaseURL: srv.URL, ClientID: "cid", ClientSecret: "csecret", WebhookSecret: "s"}
	ctx := context.Background()

	u, err := url.Parse(c.AuthURL("st"))
	if err != nil || u.Path != "/login/oauth/authorize" || u.Query().Get("client_id") != "cid" || u.Query().Get("state") != "st" || u.Query().Get("scope") != "repo,user:email" {
		t.Fatalf("auth url: %v %v", u, err)
	}

	token, id, login, err := c.ExchangeCode(ctx, "good")
	if err != nil || token != "tok" || id != 99 || login != "octo" {
		t.Fatalf("exchange: %q %d %q %v", token, id, login, err)
	}
	if _, _, _, err := c.ExchangeCode(ctx, "bad"); !errors.Is(err, ErrInvalidOAuthState) {
		t.Fatalf("bad code: %v", err)
	}

	repos, err := c.ListRepositories(ctx, "tok")
	if err != nil || len(repos) != reposPerPage+3 || repos[0].OwnerLogin != "octo" || !repos[0].Private {
		t.Fatalf("list: %d %v", len(repos), err)
	}
	if _, err := c.ListRepositories(ctx, "revoked"); !errors.Is(err, ErrTokenRejected) {
		t.Fatalf("revoked token: %v", err)
	}

	payload := []byte(`{"a":1}`)
	if !c.VerifyWebhookSignature(payload, SignPayload("s", payload)) || c.VerifyWebhookSignature(payload, SignPayload("t", payload)) {
		t.Fatal("webhook signature verification")
	}
}
