// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
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
// `/manage` REST API exposed by OpenPanel (apps/api/src/routes/manage.router.ts)
// for managing Projects, Clients (SDK keys), and References (event
// annotations) within an Organization.
//
// The provider authenticates as a root-typed Client. Organizations are
// not creatable through this surface today — that requires a dashboard
// session and lives outside the /manage API (upstream contribution
// candidate).
type OpenPanelProvider struct {
	// version is set to the provider version on release, "dev" when the
	// provider is built and ran locally, and "test" when running
	// acceptance testing.
	version string
}

// OpenPanelProviderModel describes the provider configuration HCL block.
type OpenPanelProviderModel struct {
	Host         types.String `tfsdk:"host"`
	ClientID     types.String `tfsdk:"client_id"`
	ClientSecret types.String `tfsdk:"client_secret"`
}

func (p *OpenPanelProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "openpanel"
	resp.Version = p.version
}

func (p *OpenPanelProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Terraform provider for OpenPanel (https://openpanel.dev). Manages Projects, Clients, and References within an existing Organization via the `/manage` REST API.",
		Attributes: map[string]schema.Attribute{
			"host": schema.StringAttribute{
				MarkdownDescription: "Base URL of the OpenPanel API (e.g. `https://analytics.example.com`). The provider appends `/api/manage/...` for requests. Falls back to the `OPENPANEL_HOST` environment variable.",
				Optional:            true,
			},
			"client_id": schema.StringAttribute{
				MarkdownDescription: "OpenPanel root-typed Client ID. Sent as the `openpanel-client-id` header. Falls back to `OPENPANEL_CLIENT_ID`.",
				Optional:            true,
				Sensitive:           true,
			},
			"client_secret": schema.StringAttribute{
				MarkdownDescription: "OpenPanel root-typed Client Secret (the `sec_*` value). Sent as the `openpanel-client-secret` header. Falls back to `OPENPANEL_CLIENT_SECRET`.",
				Optional:            true,
				Sensitive:           true,
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
	clientID := stringOrEnv(data.ClientID, "OPENPANEL_CLIENT_ID")
	clientSecret := stringOrEnv(data.ClientSecret, "OPENPANEL_CLIENT_SECRET")

	if host == "" {
		resp.Diagnostics.AddError("OpenPanel host is required",
			"Set `host` in the provider block or the OPENPANEL_HOST environment variable.")
	}
	if clientID == "" {
		resp.Diagnostics.AddError("OpenPanel client_id is required",
			"Set `client_id` in the provider block or the OPENPANEL_CLIENT_ID environment variable. The credential must belong to a root-typed Client.")
	}
	if clientSecret == "" {
		resp.Diagnostics.AddError("OpenPanel client_secret is required",
			"Set `client_secret` in the provider block or the OPENPANEL_CLIENT_SECRET environment variable.")
	}
	if resp.Diagnostics.HasError() {
		return
	}

	c := client.New(host, clientID, clientSecret, p.version)
	resp.ResourceData = c
	resp.DataSourceData = c
}

func (p *OpenPanelProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
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
