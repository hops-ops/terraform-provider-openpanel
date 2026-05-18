// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hops-ops/terraform-provider-openpanel/internal/client"
)

var (
	_ resource.Resource                = &OrganizationSsoConfigResource{}
	_ resource.ResourceWithImportState = &OrganizationSsoConfigResource{}
)

func NewOrganizationSsoConfigResource() resource.Resource {
	return &OrganizationSsoConfigResource{}
}

// OrganizationSsoConfigResource manages the per-organization OIDC SSO
// configuration. 1:1 with `openpanel_organization`. The plaintext
// `oidc_client_secret` is sent over TLS on writes, encrypted at rest
// server-side with AES-256-GCM, and never returned by the API on
// reads — the schema marks it Sensitive and the Read handler does
// NOT update it from server state to avoid bumping plan-drift on
// every refresh.
type OrganizationSsoConfigResource struct {
	client *client.Client
}

type OrganizationSsoConfigResourceModel struct {
	ID                        types.String `tfsdk:"id"`
	OrganizationID            types.String `tfsdk:"organization_id"`
	Provider                  types.String `tfsdk:"provider_type"`
	DisplayName               types.String `tfsdk:"display_name"`
	OidcClientID              types.String `tfsdk:"oidc_client_id"`
	OidcClientSecret          types.String `tfsdk:"oidc_client_secret"`
	OidcAuthorizationEndpoint types.String `tfsdk:"oidc_authorization_endpoint"`
	OidcTokenEndpoint         types.String `tfsdk:"oidc_token_endpoint"`
	OidcJwksURI               types.String `tfsdk:"oidc_jwks_uri"`
	EnforcedForDomains        types.List   `tfsdk:"enforced_for_domains"`
	IsRequired                types.Bool   `tfsdk:"is_required"`
	HasOidcClientSecret       types.Bool   `tfsdk:"has_oidc_client_secret"`
	CreatedAt                 types.String `tfsdk:"created_at"`
	UpdatedAt                 types.String `tfsdk:"updated_at"`
}

func (r *OrganizationSsoConfigResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_organization_sso_config"
}

func (r *OrganizationSsoConfigResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Per-organization OIDC SSO configuration. Routes `/login` for users whose email domain matches one of the configured `enforced_for_domains`, and (when `is_required = true`) blocks email/password sign-in for existing members. 1:1 with `openpanel_organization`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Server-assigned UUID of the SSO config row. Equal to the organization_id in practice but the API exposes a distinct id field.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"organization_id": schema.StringAttribute{
				MarkdownDescription: "ID of the OpenPanel Organization this SSO config belongs to. Immutable after create.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"provider_type": schema.StringAttribute{
				MarkdownDescription: "SSO provider type. Only `OIDC` is supported in v1. (Named `provider_type` rather than `provider` to avoid the Terraform-reserved `provider` block-aliased attribute name.)",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"display_name": schema.StringAttribute{
				MarkdownDescription: "User-facing label shown on the `/login` SSO button. Defaults to \"Single Sign-On\".",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"oidc_client_id": schema.StringAttribute{
				MarkdownDescription: "OIDC `client_id` issued by your IdP for this OpenPanel install.",
				Required:            true,
			},
			"oidc_client_secret": schema.StringAttribute{
				MarkdownDescription: "OIDC `client_secret`. Cleartext crosses TLS once on write, encrypted at rest server-side, and NEVER returned on read. Mark this value as sensitive in the calling Terraform config; consider sourcing it from a SecretsManager `data` source rather than literal HCL.",
				Required:            true,
				Sensitive:           true,
			},
			"oidc_authorization_endpoint": schema.StringAttribute{
				MarkdownDescription: "IdP's OAuth 2.0 authorization endpoint (e.g. `https://auth.example.com/oauth/v2/authorize`).",
				Required:            true,
			},
			"oidc_token_endpoint": schema.StringAttribute{
				MarkdownDescription: "IdP's OAuth 2.0 token endpoint (e.g. `https://auth.example.com/oauth/v2/token`).",
				Required:            true,
			},
			"oidc_jwks_uri": schema.StringAttribute{
				MarkdownDescription: "Optional JWKS endpoint for ID-token signature verification.",
				Optional:            true,
			},
			"enforced_for_domains": schema.ListAttribute{
				MarkdownDescription: "Email domains routed to this Organization's SSO at sign-in time. Example: `[\"acme.com\", \"acme.co.uk\"]`.",
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
			},
			"is_required": schema.BoolAttribute{
				MarkdownDescription: "When `true`, members of this Organization cannot sign in with email + password. They must use SSO. Test the flow end-to-end before flipping.",
				Optional:            true,
				Computed:            true,
			},
			"has_oidc_client_secret": schema.BoolAttribute{
				MarkdownDescription: "Reported by the server on every read — `true` once a secret has been written. Useful for catching drift when an out-of-band edit clears the secret.",
				Computed:            true,
			},
			"created_at": schema.StringAttribute{Computed: true},
			"updated_at": schema.StringAttribute{Computed: true},
		},
	}
}

func (r *OrganizationSsoConfigResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type",
			fmt.Sprintf("Expected *client.Client, got %T", req.ProviderData))
		return
	}
	r.client = c
}

func (r *OrganizationSsoConfigResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan OrganizationSsoConfigResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	in := planToClient(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	out, err := r.client.UpsertOrgSsoConfig(ctx, plan.OrganizationID.ValueString(), in)
	if err != nil {
		resp.Diagnostics.AddError("OpenPanel UpsertOrgSsoConfig failed", err.Error())
		return
	}

	// Server doesn't echo the secret back; carry the plan's value into
	// state so subsequent plans don't see drift on the write-only field.
	resp.Diagnostics.Append(resp.State.Set(ctx, ssoToModel(out, plan.OidcClientSecret))...)
}

func (r *OrganizationSsoConfigResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state OrganizationSsoConfigResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	out, err := r.client.GetOrgSsoConfig(ctx, state.OrganizationID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("OpenPanel GetOrgSsoConfig failed", err.Error())
		return
	}

	// Preserve the secret already in state — the server never returns
	// the cleartext on read, so a naïve overwrite would clear the
	// state's stored value and cause every reconcile to diff.
	resp.Diagnostics.Append(resp.State.Set(ctx, ssoToModel(out, state.OidcClientSecret))...)
}

func (r *OrganizationSsoConfigResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan OrganizationSsoConfigResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	in := planToClient(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	out, err := r.client.UpsertOrgSsoConfig(ctx, plan.OrganizationID.ValueString(), in)
	if err != nil {
		resp.Diagnostics.AddError("OpenPanel UpsertOrgSsoConfig (update) failed", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, ssoToModel(out, plan.OidcClientSecret))...)
}

func (r *OrganizationSsoConfigResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state OrganizationSsoConfigResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteOrgSsoConfig(ctx, state.OrganizationID.ValueString()); err != nil {
		if client.IsNotFound(err) {
			return
		}
		resp.Diagnostics.AddError("OpenPanel DeleteOrgSsoConfig failed", err.Error())
	}
}

func (r *OrganizationSsoConfigResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import by organization_id — the natural unique key. After import,
	// `oidc_client_secret` will be empty in state because the API does
	// not return it; the operator must `terraform apply` with the
	// cleartext to repopulate the write-only attribute.
	resource.ImportStatePassthroughID(ctx, path.Root("organization_id"), req, resp)
}

func planToClient(ctx context.Context, plan *OrganizationSsoConfigResourceModel, diags *diag.Diagnostics) *client.OrgSsoConfig {
	var domains []string
	if !plan.EnforcedForDomains.IsNull() && !plan.EnforcedForDomains.IsUnknown() {
		d := plan.EnforcedForDomains.ElementsAs(ctx, &domains, false)
		diags.Append(d...)
		if d.HasError() {
			return nil
		}
	}
	c := &client.OrgSsoConfig{
		OrganizationID:            plan.OrganizationID.ValueString(),
		Provider:                  valueOrDefault(plan.Provider, "OIDC"),
		DisplayName:               valueOrDefault(plan.DisplayName, ""),
		OidcClientID:              plan.OidcClientID.ValueString(),
		OidcClientSecret:          plan.OidcClientSecret.ValueString(),
		OidcAuthorizationEndpoint: plan.OidcAuthorizationEndpoint.ValueString(),
		OidcTokenEndpoint:         plan.OidcTokenEndpoint.ValueString(),
		OidcJwksUri:               plan.OidcJwksURI.ValueString(),
		EnforcedForDomains:        domains,
		IsRequired:                plan.IsRequired.ValueBool(),
	}
	return c
}

func ssoToModel(o *client.OrgSsoConfig, preservedSecret types.String) *OrganizationSsoConfigResourceModel {
	domains := make([]attr.Value, 0, len(o.EnforcedForDomains))
	for _, d := range o.EnforcedForDomains {
		domains = append(domains, types.StringValue(d))
	}
	list, _ := types.ListValue(types.StringType, domains)
	return &OrganizationSsoConfigResourceModel{
		ID:                        types.StringValue(o.ID),
		OrganizationID:            types.StringValue(o.OrganizationID),
		Provider:                  types.StringValue(o.Provider),
		DisplayName:               types.StringValue(o.DisplayName),
		OidcClientID:              types.StringValue(o.OidcClientID),
		OidcClientSecret:          preservedSecret,
		OidcAuthorizationEndpoint: types.StringValue(o.OidcAuthorizationEndpoint),
		OidcTokenEndpoint:         types.StringValue(o.OidcTokenEndpoint),
		OidcJwksURI:               types.StringValue(o.OidcJwksUri),
		EnforcedForDomains:        list,
		IsRequired:                types.BoolValue(o.IsRequired),
		HasOidcClientSecret:       types.BoolValue(o.HasOidcClientSecret),
		CreatedAt:                 types.StringValue(o.CreatedAt),
		UpdatedAt:                 types.StringValue(o.UpdatedAt),
	}
}

func valueOrDefault(v types.String, def string) string {
	if v.IsNull() || v.IsUnknown() || v.ValueString() == "" {
		return def
	}
	return v.ValueString()
}
