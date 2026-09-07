package oauth

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/domain"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func jsonResponse(body string) *http.Response {
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}
}

func TestGoogleExchangeUsesPKCEAndMapsProfile(t *testing.T) {
	var tokenCalls, profileCalls int
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Host {
		case "oauth2.googleapis.com":
			tokenCalls++
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			if r.Form.Get("code_verifier") != "verifier" || r.Form.Get("code") != "code" {
				t.Fatalf("bad exchange form: %v", r.Form)
			}
			return jsonResponse(`{"access_token":"access","token_type":"Bearer"}`), nil
		case "www.googleapis.com":
			profileCalls++
			if r.Header.Get("Authorization") != "Bearer access" {
				t.Fatal("missing access token")
			}
			return jsonResponse(`{"id":"123","email":"reader@example.com","verified_email":true,"name":"Reader","picture":"https://example.com/avatar.jpg"}`), nil
		default:
			t.Fatalf("unexpected request: %s", r.URL)
			return nil, errors.New("unexpected request")
		}
	})}
	gateway := New(NewGoogleOAuthConfig("client", "secret", "https://example.com/portfolio/v1"), nil, client)
	start, err := gateway.Start(OAuthProviderGoogle)
	if err != nil {
		t.Fatal(err)
	}
	authURL, _ := url.Parse(start.AuthURL)
	if start.State == "" || start.Verifier == "" || authURL.Query().Get("code_challenge_method") != "S256" {
		t.Fatalf("invalid PKCE start: %+v", start)
	}
	profile, err := gateway.Exchange(context.Background(), OAuthProviderGoogle, "code", "state", "verifier")
	if err != nil {
		t.Fatal(err)
	}
	if profile.Email != "reader@example.com" || profile.ProviderID != "123" || !profile.EmailVerified || tokenCalls != 1 || profileCalls != 1 {
		t.Fatalf("profile=%+v, calls=%d/%d", profile, tokenCalls, profileCalls)
	}
}

func TestGitHubDevicePendingStatesDoNotFetchProfile(t *testing.T) {
	for _, code := range []string{"authorization_pending", "slow_down", "access_denied"} {
		t.Run(code, func(t *testing.T) {
			calls := 0
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.URL.String() != "https://github.com/login/oauth/access_token" {
					t.Fatalf("unexpected request: %s", r.URL)
				}
				return jsonResponse(`{"error":"` + code + `"}`), nil
			})}
			gateway := New(nil, NewGitHubOAuthConfig("client", "secret", "https://example.com"), client)
			_, status, err := gateway.PollDevice(context.Background(), "device-code")
			switch code {
			case "authorization_pending":
				if status != DeviceAuthPending || err != nil {
					t.Fatalf("%s, %v", status, err)
				}
			case "slow_down":
				if status != DeviceAuthSlowDown || err != nil {
					t.Fatalf("%s, %v", status, err)
				}
			case "access_denied":
				if !errors.Is(err, domain.ErrBadRequest) {
					t.Fatalf("error=%v", err)
				}
			}
			if calls != 1 {
				t.Fatalf("calls=%d", calls)
			}
		})
	}
}
