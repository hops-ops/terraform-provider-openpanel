// SPDX-License-Identifier: MPL-2.0

// Package client is a thin Go HTTP client for OpenPanel's `/manage`
// REST API (https://openpanel.dev/docs/api/manage).
//
// Authentication is the `openpanel-client-id` + `openpanel-client-secret`
// header pair, scoped to a root-typed Client (see
// apps/api/src/utils/auth.ts::validateManageRequest in the OpenPanel
// upstream).
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	defaultTimeout = 30 * time.Second
	managePrefix   = "/api/manage"
)

// Client is the OpenPanel /manage REST client. Authentication is
// pluggable via Authorizer — see auth.go for the three supported
// auth modes (Client-pair headers, OIDC client_credentials, static
// Bearer).
type Client struct {
	host      string
	auth      Authorizer
	userAgent string
	http      *http.Client
}

// New constructs a Client. host is the base URL of the OpenPanel API
// (e.g. https://analytics.example.com); the client appends
// /api/manage/* to each request.
func New(host string, auth Authorizer, providerVersion string) *Client {
	httpClient := &http.Client{
		Timeout: defaultTimeout,
	}
	// OIDC authorizer needs an http.Client for its discovery + token
	// fetch. Share the same client so token-endpoint calls use the
	// provider's timeout settings.
	if oidc, ok := auth.(*OIDCClientCredentialsAuth); ok && oidc.http == nil {
		oidc.http = httpClient
	}
	return &Client{
		host:      strings.TrimRight(host, "/"),
		auth:      auth,
		userAgent: fmt.Sprintf("terraform-provider-openpanel/%s", providerVersion),
		http:      httpClient,
	}
}

// apiError surfaces a non-2xx response with the body included so callers
// can write useful diagnostics.
type apiError struct {
	StatusCode int
	Body       string
	Method     string
	Path       string
}

func (e *apiError) Error() string {
	return fmt.Sprintf("openpanel: %s %s returned %d: %s", e.Method, e.Path, e.StatusCode, e.Body)
}

// IsNotFound reports whether err is a 404 from the OpenPanel API. Used
// in resource Read methods to clear state when a resource was deleted
// out-of-band.
func IsNotFound(err error) bool {
	if err == nil {
		return false
	}
	ae, ok := err.(*apiError)
	return ok && ae.StatusCode == http.StatusNotFound
}

// do executes an HTTP request against /api/manage/<path>, marshalling
// body as JSON (if non-nil) and decoding the response into out (if
// non-nil).
func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	url := c.host + managePrefix + path

	var reqBody io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(buf)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	if err := c.auth.Authorize(ctx, req); err != nil {
		return fmt.Errorf("authorize request: %w", err)
	}
	req.Header.Set("user-agent", c.userAgent)
	if body != nil {
		req.Header.Set("content-type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &apiError{
			StatusCode: resp.StatusCode,
			Body:       string(respBody),
			Method:     method,
			Path:       managePrefix + path,
		}
	}

	if out != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("decode response body: %w", err)
		}
	}

	return nil
}

// ---------------------------------------------------------------------
// Projects
// ---------------------------------------------------------------------

// Project mirrors the OpenPanel project shape returned by /manage/projects.
type Project struct {
	ID             string  `json:"id,omitempty"`
	OrganizationID string  `json:"organizationId,omitempty"`
	Name           string  `json:"name"`
	Domain         *string `json:"domain,omitempty"`
	CORS           *string `json:"cors,omitempty"`
	CreatedAt      string  `json:"createdAt,omitempty"`
	UpdatedAt      string  `json:"updatedAt,omitempty"`
}

func (c *Client) CreateProject(ctx context.Context, p *Project) (*Project, error) {
	var out Project
	if err := c.do(ctx, http.MethodPost, "/projects", p, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) GetProject(ctx context.Context, id string) (*Project, error) {
	var out Project
	if err := c.do(ctx, http.MethodGet, "/projects/"+id, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) UpdateProject(ctx context.Context, id string, p *Project) (*Project, error) {
	var out Project
	if err := c.do(ctx, http.MethodPatch, "/projects/"+id, p, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeleteProject(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/projects/"+id, nil, nil)
}

// ---------------------------------------------------------------------
// Clients (SDK keys)
// ---------------------------------------------------------------------

// ClientType is one of "read" | "write" | "root". Only "root" clients
// can call /manage, so creating root clients is sensitive — the secret
// is returned exactly once at Create time.
type ClientType string

const (
	ClientTypeRead  ClientType = "read"
	ClientTypeWrite ClientType = "write"
	ClientTypeRoot  ClientType = "root"
)

// SDKClient is named to avoid colliding with the package name. Mirrors
// the shape returned by /manage/clients.
//
// CORS is space-separated origins for write clients (the JS SDK CORS
// allowlist). Secret is populated only on Create — subsequent Read /
// Update responses elide it.
type SDKClient struct {
	ID        string     `json:"id,omitempty"`
	Name      string     `json:"name"`
	ProjectID string     `json:"projectId"`
	Type      ClientType `json:"type"`
	CORS      *string    `json:"cors,omitempty"`
	Secret    *string    `json:"secret,omitempty"` // sec_* — Create-time only
	CreatedAt string     `json:"createdAt,omitempty"`
	UpdatedAt string     `json:"updatedAt,omitempty"`
}

func (c *Client) CreateClient(ctx context.Context, cl *SDKClient) (*SDKClient, error) {
	var out SDKClient
	if err := c.do(ctx, http.MethodPost, "/clients", cl, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) GetClient(ctx context.Context, id string) (*SDKClient, error) {
	var out SDKClient
	if err := c.do(ctx, http.MethodGet, "/clients/"+id, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) UpdateClient(ctx context.Context, id string, cl *SDKClient) (*SDKClient, error) {
	var out SDKClient
	if err := c.do(ctx, http.MethodPatch, "/clients/"+id, cl, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeleteClient(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/clients/"+id, nil, nil)
}

// ---------------------------------------------------------------------
// References (event annotations)
// ---------------------------------------------------------------------

// Reference annotates an event in the analytics timeline (e.g. a deploy,
// a marketing campaign launch). Mirrors /manage/references.
type Reference struct {
	ID          string  `json:"id,omitempty"`
	ProjectID   string  `json:"projectId"`
	Title       string  `json:"title"`
	Description *string `json:"description,omitempty"`
	Date        string  `json:"date"` // RFC 3339
	CreatedAt   string  `json:"createdAt,omitempty"`
	UpdatedAt   string  `json:"updatedAt,omitempty"`
}

func (c *Client) CreateReference(ctx context.Context, r *Reference) (*Reference, error) {
	var out Reference
	if err := c.do(ctx, http.MethodPost, "/references", r, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) GetReference(ctx context.Context, id string) (*Reference, error) {
	var out Reference
	if err := c.do(ctx, http.MethodGet, "/references/"+id, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) UpdateReference(ctx context.Context, id string, r *Reference) (*Reference, error) {
	var out Reference
	if err := c.do(ctx, http.MethodPatch, "/references/"+id, r, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeleteReference(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/references/"+id, nil, nil)
}
