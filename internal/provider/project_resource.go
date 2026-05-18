// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hops-ops/terraform-provider-openpanel/internal/client"
)

var (
	_ resource.Resource                = &ProjectResource{}
	_ resource.ResourceWithImportState = &ProjectResource{}
)

func NewProjectResource() resource.Resource {
	return &ProjectResource{}
}

// ProjectResource manages an OpenPanel Project within the Organization
// the provider's root Client belongs to. Projects scope Clients
// (SDK keys) and References (timeline annotations) underneath them.
type ProjectResource struct {
	client *client.Client
}

type ProjectResourceModel struct {
	ID             types.String `tfsdk:"id"`
	OrganizationID types.String `tfsdk:"organization_id"`
	Name           types.String `tfsdk:"name"`
	Domain         types.String `tfsdk:"domain"`
	CORS           types.List   `tfsdk:"cors"`
	CreatedAt      types.String `tfsdk:"created_at"`
	UpdatedAt      types.String `tfsdk:"updated_at"`
}

func (r *ProjectResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_project"
}

func (r *ProjectResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "An OpenPanel Project. Projects scope SDK clients and event references; they are the unit a tenant app integrates against.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Project ID assigned by OpenPanel.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"organization_id": schema.StringAttribute{
				MarkdownDescription: "ID of the Organization this Project belongs to. Inferred from the provider's root Client credential.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Human-readable Project name. Shown in the OpenPanel dashboard.",
				Required:            true,
			},
			"domain": schema.StringAttribute{
				MarkdownDescription: "Primary site domain associated with the Project. Optional metadata.",
				Optional:            true,
			},
			"cors": schema.ListAttribute{
				MarkdownDescription: "List of allowed origins for the Project's write clients (the JS SDK CORS allowlist).",
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
			},
			"created_at": schema.StringAttribute{
				MarkdownDescription: "RFC 3339 timestamp.",
				Computed:            true,
			},
			"updated_at": schema.StringAttribute{
				MarkdownDescription: "RFC 3339 timestamp.",
				Computed:            true,
			},
		},
	}
}

func (r *ProjectResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *ProjectResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ProjectResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	in := &client.Project{
		Name:   plan.Name.ValueString(),
		Domain: stringPtrOrNil(plan.Domain),
		CORS:   listToStrings(ctx, plan.CORS),
	}

	out, err := r.client.CreateProject(ctx, in)
	if err != nil {
		resp.Diagnostics.AddError("OpenPanel CreateProject failed", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, projectToModel(out))...)
}

func (r *ProjectResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ProjectResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	out, err := r.client.GetProject(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("OpenPanel GetProject failed", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, projectToModel(out))...)
}

func (r *ProjectResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan ProjectResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	in := &client.Project{
		Name:   plan.Name.ValueString(),
		Domain: stringPtrOrNil(plan.Domain),
		CORS:   listToStrings(ctx, plan.CORS),
	}

	out, err := r.client.UpdateProject(ctx, plan.ID.ValueString(), in)
	if err != nil {
		resp.Diagnostics.AddError("OpenPanel UpdateProject failed", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, projectToModel(out))...)
}

func (r *ProjectResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ProjectResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteProject(ctx, state.ID.ValueString()); err != nil {
		if client.IsNotFound(err) {
			return
		}
		resp.Diagnostics.AddError("OpenPanel DeleteProject failed", err.Error())
	}
}

func (r *ProjectResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func projectToModel(p *client.Project) *ProjectResourceModel {
	return &ProjectResourceModel{
		ID:             types.StringValue(p.ID),
		OrganizationID: types.StringValue(p.OrganizationID),
		Name:           types.StringValue(p.Name),
		Domain:         stringValueOrNull(p.Domain),
		CORS:           stringsToList(p.CORS),
		CreatedAt:      types.StringValue(p.CreatedAt),
		UpdatedAt:      types.StringValue(p.UpdatedAt),
	}
}

// listToStrings extracts a Go []string from a types.List of types.StringType.
// Returns nil when the list is null/unknown.
func listToStrings(ctx context.Context, l types.List) []string {
	if l.IsNull() || l.IsUnknown() {
		return nil
	}
	var out []string
	_ = l.ElementsAs(ctx, &out, false)
	return out
}

// stringsToList converts a Go []string to a types.List of strings.
// `nil` becomes an empty list rather than null to match what OpenPanel
// returns when no CORS origins are configured.
func stringsToList(ss []string) types.List {
	if ss == nil {
		ss = []string{}
	}
	vals := make([]attr.Value, len(ss))
	for i, s := range ss {
		vals[i] = types.StringValue(s)
	}
	l, _ := types.ListValue(types.StringType, vals)
	return l
}

func stringPtrOrNil(v types.String) *string {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	s := v.ValueString()
	return &s
}

func stringValueOrNull(s *string) types.String {
	if s == nil {
		return types.StringNull()
	}
	return types.StringValue(*s)
}
