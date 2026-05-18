// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Authorizer attaches credentials to outgoing /manage requests.
// Three implementations exist:
//   - ClientPairAuth: openpanel-client-id / openpanel-client-secret
//     headers (existing /manage auth surface). Works with any
//     upstream OpenPanel install once you have a root-typed Client.
//   - StaticBearerAuth: a pre-obtained Bearer token, attached as-is.
//     Useful when an external tool already manages JWT issuance and
//     hands the provider a token via env or file.
//   - OIDCClientCredentialsAuth: runs the OIDC client_credentials
//     grant against the configured issuer, caches the JWT until
//     ~60s before expiry, refreshes transparently. Requires the
//     openpanel-app fork (or any OpenPanel install) configured with
//     ADMIN_OIDC_ISSUER to validate the resulting Bearer.
type Authorizer interface {
	Authorize(ctx context.Context, req *http.Request) error
}

// ---------------------------------------------------------------------
// Client-pair (existing /manage auth)
// ---------------------------------------------------------------------

type ClientPairAuth struct {
	ClientID     string
	ClientSecret string
}

func (a *ClientPairAuth) Authorize(_ context.Context, req *http.Request) error {
	req.Header.Set("openpanel-client-id", a.ClientID)
	req.Header.Set("openpanel-client-secret", a.ClientSecret)
	return nil
}

// ---------------------------------------------------------------------
// Static Bearer (pre-obtained JWT)
// ---------------------------------------------------------------------

type StaticBearerAuth struct {
	Token string
}

func (a *StaticBearerAuth) Authorize(_ context.Context, req *http.Request) error {
	req.Header.Set("Authorization", "Bearer "+a.Token)
	return nil
}

// ---------------------------------------------------------------------
// OIDC client_credentials grant
// ---------------------------------------------------------------------

// OIDCClientCredentialsAuth performs the OAuth 2.0 client_credentials
// grant against an OIDC issuer's token endpoint (discovered via
// /.well-known/openid-configuration) and attaches the resulting JWT
// as a Bearer token on every request. The token is cached in memory
// until ~60s before its expiry, at which point the next request
// transparently refreshes it.
type OIDCClientCredentialsAuth struct {
	Issuer       string // base URL, e.g. https://auth.example.com
	ClientID     string
	ClientSecret string
	Audience     string // optional; some IdPs require it as a request param
	Scopes       []string

	http *http.Client

	mu            sync.Mutex
	tokenEndpoint string
	cachedToken   string
	cachedExpiry  time.Time
}

func NewOIDCClientCredentialsAuth(httpClient *http.Client, issuer, clientID, clientSecret, audience string) *OIDCClientCredentialsAuth {
	return &OIDCClientCredentialsAuth{
		Issuer:       strings.TrimRight(issuer, "/"),
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Audience:     audience,
		http:         httpClient,
	}
}

func (a *OIDCClientCredentialsAuth) Authorize(ctx context.Context, req *http.Request) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	now := time.Now()
	if a.cachedToken == "" || now.After(a.cachedExpiry.Add(-60*time.Second)) {
		if err := a.refreshLocked(ctx); err != nil {
			return fmt.Errorf("openpanel: refresh OIDC access token: %w", err)
		}
	}
	req.Header.Set("Authorization", "Bearer "+a.cachedToken)
	return nil
}

func (a *OIDCClientCredentialsAuth) refreshLocked(ctx context.Context) error {
	if a.tokenEndpoint == "" {
		if err := a.discoverLocked(ctx); err != nil {
			return err
		}
	}

	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	if a.Audience != "" {
		form.Set("audience", a.Audience)
	}
	if len(a.Scopes) > 0 {
		form.Set("scope", strings.Join(a.Scopes, " "))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.tokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("content-type", "application/x-www-form-urlencoded")
	req.Header.Set("accept", "application/json")
	req.SetBasicAuth(url.QueryEscape(a.ClientID), url.QueryEscape(a.ClientSecret))

	resp, err := a.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("token endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	var out struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		TokenType   string `json:"token_type"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return fmt.Errorf("decode token response: %w", err)
	}
	if out.AccessToken == "" {
		return errors.New("token endpoint returned empty access_token")
	}

	expiresIn := out.ExpiresIn
	if expiresIn <= 0 {
		// Reasonable default for IdPs that omit expires_in.
		expiresIn = 3600
	}
	a.cachedToken = out.AccessToken
	a.cachedExpiry = time.Now().Add(time.Duration(expiresIn) * time.Second)
	return nil
}

// discoverLocked fetches /.well-known/openid-configuration to find
// the token endpoint. Called once per provider instance and cached.
func (a *OIDCClientCredentialsAuth) discoverLocked(ctx context.Context) error {
	disco, err := url.JoinPath(a.Issuer, "/.well-known/openid-configuration")
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, disco, nil)
	if err != nil {
		return err
	}
	resp, err := a.http.Do(req)
	if err != nil {
		return fmt.Errorf("fetch %s: %w", disco, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("discovery returned %d: %s", resp.StatusCode, string(body))
	}
	var doc struct {
		TokenEndpoint string `json:"token_endpoint"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return fmt.Errorf("decode discovery: %w", err)
	}
	if doc.TokenEndpoint == "" {
		return errors.New("discovery doc missing token_endpoint")
	}
	a.tokenEndpoint = doc.TokenEndpoint
	return nil
}
