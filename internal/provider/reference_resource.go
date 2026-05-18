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
	_ resource.Resource                = &ReferenceResource{}
	_ resource.ResourceWithImportState = &ReferenceResource{}
)

func NewReferenceResource() resource.Resource {
	return &ReferenceResource{}
}

// ReferenceResource manages an OpenPanel Reference — a timeline
// annotation surfaced alongside event data in dashboards (deploys,
// campaign launches, incidents, etc.).
type ReferenceResource struct {
	client *client.Client
}

type ReferenceResourceModel struct {
	ID          types.String `tfsdk:"id"`
	ProjectID   types.String `tfsdk:"project_id"`
	Title       types.String `tfsdk:"title"`
	Description types.String `tfsdk:"description"`
	Date        types.String `tfsdk:"date"`
	CreatedAt   types.String `tfsdk:"created_at"`
	UpdatedAt   types.String `tfsdk:"updated_at"`
}

func (r *ReferenceResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_reference"
}

func (r *ReferenceResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A Reference annotates an event in the OpenPanel timeline (deploy, campaign, incident). Surfaces on dashboards alongside the analytics.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"project_id": schema.StringAttribute{
				MarkdownDescription: "Owning Project ID.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"title": schema.StringAttribute{
				Required: true,
			},
			"description": schema.StringAttribute{
				Optional: true,
			},
			"date": schema.StringAttribute{
				MarkdownDescription: "RFC 3339 timestamp the reference annotates.",
				Required:            true,
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

func (r *ReferenceResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *ReferenceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ReferenceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	in := &client.Reference{
		ProjectID:   plan.ProjectID.ValueString(),
		Title:       plan.Title.ValueString(),
		Description: stringPtrOrNil(plan.Description),
		Date:        plan.Date.ValueString(),
	}

	out, err := r.client.CreateReference(ctx, in)
	if err != nil {
		resp.Diagnostics.AddError("OpenPanel CreateReference failed", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, referenceToModel(out))...)
}

func (r *ReferenceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ReferenceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	out, err := r.client.GetReference(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("OpenPanel GetReference failed", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, referenceToModel(out))...)
}

func (r *ReferenceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan ReferenceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	in := &client.Reference{
		ProjectID:   plan.ProjectID.ValueString(),
		Title:       plan.Title.ValueString(),
		Description: stringPtrOrNil(plan.Description),
		Date:        plan.Date.ValueString(),
	}

	out, err := r.client.UpdateReference(ctx, plan.ID.ValueString(), in)
	if err != nil {
		resp.Diagnostics.AddError("OpenPanel UpdateReference failed", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, referenceToModel(out))...)
}

func (r *ReferenceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ReferenceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteReference(ctx, state.ID.ValueString()); err != nil {
		if client.IsNotFound(err) {
			return
		}
		resp.Diagnostics.AddError("OpenPanel DeleteReference failed", err.Error())
	}
}

func (r *ReferenceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func referenceToModel(r *client.Reference) *ReferenceResourceModel {
	return &ReferenceResourceModel{
		ID:          types.StringValue(r.ID),
		ProjectID:   types.StringValue(r.ProjectID),
		Title:       types.StringValue(r.Title),
		Description: stringValueOrNull(r.Description),
		Date:        types.StringValue(r.Date),
		CreatedAt:   types.StringValue(r.CreatedAt),
		UpdatedAt:   types.StringValue(r.UpdatedAt),
	}
}
