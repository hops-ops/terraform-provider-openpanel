// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hops-ops/terraform-provider-openpanel/internal/client"
)

var (
	_ resource.Resource                = &OrganizationResource{}
	_ resource.ResourceWithImportState = &OrganizationResource{}
)

func NewOrganizationResource() resource.Resource {
	return &OrganizationResource{}
}

// OrganizationResource manages an OpenPanel Organization — the top-level
// tenant-isolation primitive. Projects, Clients, References, and Member
// invitations all live within an Organization.
//
// Requires the upstream `/manage/organizations` endpoints introduced in
// Openpanel-dev/openpanel#371 (currently only available on the
// hops-ops/openpanel-app fork). Older OpenPanel installs will 404 on
// this resource.
type OrganizationResource struct {
	client *client.Client
}

type OrganizationResourceModel struct {
	ID        types.String `tfsdk:"id"`
	Name      types.String `tfsdk:"name"`
	Timezone  types.String `tfsdk:"timezone"`
	CreatedAt types.String `tfsdk:"created_at"`
	UpdatedAt types.String `tfsdk:"updated_at"`
}

func (r *OrganizationResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_organization"
}

func (r *OrganizationResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "An OpenPanel Organization. The top-level tenant primitive; scopes Projects, Clients, References, and Members.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Organization ID. Slugified from the name when not specified at create-time.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Human-readable Organization name. Shown in the OpenPanel dashboard.",
				Required:            true,
			},
			"timezone": schema.StringAttribute{
				MarkdownDescription: "IANA timezone (e.g. `America/Chicago`). Drives the dashboard's default reporting window.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"created_at": schema.StringAttribute{
				Computed: true,
			},
			"updated_at": schema.StringAttribute{
				Computed: true,
			},
		},
	}
}

func (r *OrganizationResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *OrganizationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan OrganizationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	in := &client.Organization{
		Name:     plan.Name.ValueString(),
		Timezone: stringPtrOrNil(plan.Timezone),
	}

	out, err := r.client.CreateOrganization(ctx, in)
	if err != nil {
		resp.Diagnostics.AddError("OpenPanel CreateOrganization failed", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, organizationToModel(out))...)
}

func (r *OrganizationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state OrganizationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	out, err := r.client.GetOrganization(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("OpenPanel GetOrganization failed", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, organizationToModel(out))...)
}

func (r *OrganizationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan OrganizationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	in := &client.Organization{
		Name:     plan.Name.ValueString(),
		Timezone: stringPtrOrNil(plan.Timezone),
	}

	out, err := r.client.UpdateOrganization(ctx, plan.ID.ValueString(), in)
	if err != nil {
		resp.Diagnostics.AddError("OpenPanel UpdateOrganization failed", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, organizationToModel(out))...)
}

func (r *OrganizationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state OrganizationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteOrganization(ctx, state.ID.ValueString()); err != nil {
		if client.IsNotFound(err) {
			return
		}
		resp.Diagnostics.AddError("OpenPanel DeleteOrganization failed", err.Error())
	}
}

func (r *OrganizationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func organizationToModel(o *client.Organization) *OrganizationResourceModel {
	return &OrganizationResourceModel{
		ID:        types.StringValue(o.ID),
		Name:      types.StringValue(o.Name),
		Timezone:  stringValueOrNull(o.Timezone),
		CreatedAt: types.StringValue(o.CreatedAt),
		UpdatedAt: types.StringValue(o.UpdatedAt),
	}
}
