// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/function"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hops-ops/terraform-provider-openpanel/internal/client"
)

// Ensure OpenPanelProvider satisfies the provider interface.
var _ provider.Provider = &OpenPanelProvider{}

// OpenPanelProvider is the OpenPanel Terraform provider. It wraps the
// `/manage` REST API exposed by OpenPanel.
//
// Three authentication modes are supported (any one required):
//
//  1. Client-pair  — root-typed Client ID + Secret. The existing
//     OpenPanel /manage auth; works against any
//     OpenPanel install once you've minted a root
//     Client through the dashboard.
//
//  2. OIDC client_credentials — the provider runs the OAuth2
//     client_credentials grant against the configured
//     issuer, caches the JWT, refreshes on expiry.
//     Requires the openpanel-app fork (or any future
//     upstream release) configured with
//     ADMIN_OIDC_ISSUER on the api pod.
//
//  3. Static Bearer  — operator supplies a pre-obtained JWT (e.g.
//     from another tool / a secret file). The
//     provider passes it through unchanged.
type OpenPanelProvider struct {
	// version is set to the provider version on release, "dev" when the
	// provider is built and ran locally, and "test" when running
	// acceptance testing.
	version string
}

// OIDCAuthModel mirrors the nested `oidc { … }` block.
type OIDCAuthModel struct {
	Issuer       types.String `tfsdk:"issuer"`
	ClientID     types.String `tfsdk:"client_id"`
	ClientSecret types.String `tfsdk:"client_secret"`
	Audience     types.String `tfsdk:"audience"`
}

// OpenPanelProviderModel describes the provider configuration HCL block.
type OpenPanelProviderModel struct {
	Host         types.String   `tfsdk:"host"`
	ClientID     types.String   `tfsdk:"client_id"`
	ClientSecret types.String   `tfsdk:"client_secret"`
	Token        types.String   `tfsdk:"token"`
	OIDC         *OIDCAuthModel `tfsdk:"oidc"`
}

func (p *OpenPanelProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "openpanel"
	resp.Version = p.version
}

func (p *OpenPanelProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Terraform provider for OpenPanel (https://openpanel.dev). Manages Projects, Clients, and References via the `/manage` REST API.",
		Attributes: map[string]schema.Attribute{
			"host": schema.StringAttribute{
				MarkdownDescription: "Base URL of the OpenPanel API (e.g. `https://analytics.example.com`). The provider appends `/api/manage/...` for requests. Falls back to the `OPENPANEL_HOST` environment variable.",
				Optional:            true,
			},

			// Auth mode 1 — Client-pair
			"client_id": schema.StringAttribute{
				MarkdownDescription: "OpenPanel root-typed Client ID. Sent as the `openpanel-client-id` header. Falls back to `OPENPANEL_CLIENT_ID`. Used when `oidc` and `token` are not configured.",
				Optional:            true,
				Sensitive:           true,
			},
			"client_secret": schema.StringAttribute{
				MarkdownDescription: "OpenPanel root-typed Client Secret (the `sec_*` value). Sent as the `openpanel-client-secret` header. Falls back to `OPENPANEL_CLIENT_SECRET`.",
				Optional:            true,
				Sensitive:           true,
			},

			// Auth mode 3 — Static Bearer
			"token": schema.StringAttribute{
				MarkdownDescription: "Static OIDC Bearer token. The provider passes this through as `Authorization: Bearer <token>` without refreshing. Falls back to `OPENPANEL_TOKEN`. Use this when an external tool already manages JWT issuance.",
				Optional:            true,
				Sensitive:           true,
			},
		},
		Blocks: map[string]schema.Block{
			// Auth mode 2 — OIDC client_credentials grant
			"oidc": schema.SingleNestedBlock{
				MarkdownDescription: "Authenticate via the OAuth2 `client_credentials` grant against an OIDC issuer. The provider mints a JWT, caches it until ~60s before expiry, then refreshes transparently. Requires the OpenPanel api pod to have `ADMIN_OIDC_ISSUER` configured.",
				Attributes: map[string]schema.Attribute{
					"issuer": schema.StringAttribute{
						MarkdownDescription: "OIDC issuer base URL (e.g. `https://auth.example.com`). The provider fetches `<issuer>/.well-known/openid-configuration` to discover the token endpoint. Falls back to `OPENPANEL_OIDC_ISSUER`.",
						Optional:            true,
					},
					"client_id": schema.StringAttribute{
						MarkdownDescription: "OIDC ServiceUser client ID. Falls back to `OPENPANEL_OIDC_CLIENT_ID`.",
						Optional:            true,
						Sensitive:           true,
					},
					"client_secret": schema.StringAttribute{
						MarkdownDescription: "OIDC ServiceUser client secret. Falls back to `OPENPANEL_OIDC_CLIENT_SECRET`.",
						Optional:            true,
						Sensitive:           true,
					},
					"audience": schema.StringAttribute{
						MarkdownDescription: "Requested audience for the issued JWT. Must match the OpenPanel api pod's `ADMIN_OIDC_AUDIENCE`. Falls back to `OPENPANEL_OIDC_AUDIENCE`.",
						Optional:            true,
					},
				},
			},
		},
	}
}

func (p *OpenPanelProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data OpenPanelProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	host := stringOrEnv(data.Host, "OPENPANEL_HOST")
	if host == "" {
		resp.Diagnostics.AddError("OpenPanel host is required",
			"Set `host` in the provider block or the OPENPANEL_HOST environment variable.")
		return
	}

	auth, err := buildAuthorizer(data)
	if err != nil {
		resp.Diagnostics.AddError("OpenPanel authentication configuration error", err.Error())
		return
	}

	c := client.New(host, auth, p.version)
	resp.ResourceData = c
	resp.DataSourceData = c
}

// buildAuthorizer picks one of the three auth modes based on what the
// HCL block + environment supply. Precedence:
//  1. Static `token` / OPENPANEL_TOKEN
//  2. `oidc { … }` block / OPENPANEL_OIDC_* env
//  3. `client_id` + `client_secret` / OPENPANEL_CLIENT_* env
//
// Exactly one mode must be fully configured.
func buildAuthorizer(data OpenPanelProviderModel) (client.Authorizer, error) {
	token := stringOrEnv(data.Token, "OPENPANEL_TOKEN")
	if token != "" {
		return &client.StaticBearerAuth{Token: token}, nil
	}

	// OIDC block — resolve each field with env fallback
	var oidcIssuer, oidcClientID, oidcClientSecret, oidcAudience string
	if data.OIDC != nil {
		oidcIssuer = stringOrEnv(data.OIDC.Issuer, "OPENPANEL_OIDC_ISSUER")
		oidcClientID = stringOrEnv(data.OIDC.ClientID, "OPENPANEL_OIDC_CLIENT_ID")
		oidcClientSecret = stringOrEnv(data.OIDC.ClientSecret, "OPENPANEL_OIDC_CLIENT_SECRET")
		oidcAudience = stringOrEnv(data.OIDC.Audience, "OPENPANEL_OIDC_AUDIENCE")
	} else {
		// Allow env-only configuration even without an HCL block, so
		// CI / .envrc setups don't have to template the block in.
		oidcIssuer = os.Getenv("OPENPANEL_OIDC_ISSUER")
		oidcClientID = os.Getenv("OPENPANEL_OIDC_CLIENT_ID")
		oidcClientSecret = os.Getenv("OPENPANEL_OIDC_CLIENT_SECRET")
		oidcAudience = os.Getenv("OPENPANEL_OIDC_AUDIENCE")
	}
	if oidcIssuer != "" || oidcClientID != "" {
		if oidcIssuer == "" || oidcClientID == "" || oidcClientSecret == "" {
			return nil, fmt.Errorf("oidc auth requires issuer + client_id + client_secret (audience optional)")
		}
		return client.NewOIDCClientCredentialsAuth(nil, oidcIssuer, oidcClientID, oidcClientSecret, oidcAudience), nil
	}

	// Client-pair fallback
	clientID := stringOrEnv(data.ClientID, "OPENPANEL_CLIENT_ID")
	clientSecret := stringOrEnv(data.ClientSecret, "OPENPANEL_CLIENT_SECRET")
	if clientID == "" || clientSecret == "" {
		return nil, fmt.Errorf("no authentication configured — set one of: (client_id + client_secret), oidc { ... }, or token")
	}
	return &client.ClientPairAuth{ClientID: clientID, ClientSecret: clientSecret}, nil
}

func (p *OpenPanelProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewOrganizationResource,
		NewOrganizationSsoConfigResource,
		NewProjectResource,
		NewClientResource,
		NewReferenceResource,
	}
}

func (p *OpenPanelProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{}
}

func (p *OpenPanelProvider) Functions(ctx context.Context) []func() function.Function {
	return []func() function.Function{}
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &OpenPanelProvider{
			version: version,
		}
	}
}

// stringOrEnv returns the configured value when known, else the env
// fallback, else "".
func stringOrEnv(v types.String, envKey string) string {
	if !v.IsNull() && !v.IsUnknown() {
		return v.ValueString()
	}
	return os.Getenv(envKey)
}
