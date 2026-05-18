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
	_ resource.Resource                = &ClientResource{}
	_ resource.ResourceWithImportState = &ClientResource{}
)

func NewClientResource() resource.Resource {
	return &ClientResource{}
}

// ClientResource manages an OpenPanel Client (the SDK credential pair
// — a `clientId` and a `sec_*` secret). The secret is returned exactly
// once at create time and is unrecoverable thereafter; replacing it
// requires destroying and re-creating the resource.
type ClientResource struct {
	client *client.Client
}

type ClientResourceModel struct {
	ID        types.String `tfsdk:"id"`
	ProjectID types.String `tfsdk:"project_id"`
	Name      types.String `tfsdk:"name"`
	Type      types.String `tfsdk:"type"`
	CORS      types.String `tfsdk:"cors"`
	Secret    types.String `tfsdk:"secret"`
	CreatedAt types.String `tfsdk:"created_at"`
	UpdatedAt types.String `tfsdk:"updated_at"`
}

func (r *ClientResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_client"
}

func (r *ClientResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "An OpenPanel SDK credential (Client). `type=write` is what your front-end SDK ships with; `type=read` reads analytics out via the API; `type=root` can call `/manage`. The secret is returned only on create.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Client ID assigned by OpenPanel. Used as the SDK's `clientId` and the `openpanel-client-id` header value.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"project_id": schema.StringAttribute{
				MarkdownDescription: "Owning Project ID. Required.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Human-readable name shown in the OpenPanel dashboard.",
				Required:            true,
			},
			"type": schema.StringAttribute{
				MarkdownDescription: "One of `read`, `write`, `root`. Drives which API surfaces accept this Client. Immutable after create.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"cors": schema.StringAttribute{
				MarkdownDescription: "Space-separated allowed origins (write clients only).",
				Optional:            true,
			},
			"secret": schema.StringAttribute{
				MarkdownDescription: "The `sec_*` secret. Populated only at create-time; subsequent reads return null.",
				Computed:            true,
				Sensitive:           true,
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

func (r *ClientResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *ClientResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ClientResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	in := &client.SDKClient{
		Name:      plan.Name.ValueString(),
		ProjectID: plan.ProjectID.ValueString(),
		Type:      client.ClientType(plan.Type.ValueString()),
		CORS:      stringPtrOrNil(plan.CORS),
	}

	out, err := r.client.CreateClient(ctx, in)
	if err != nil {
		resp.Diagnostics.AddError("OpenPanel CreateClient failed", err.Error())
		return
	}

	// Preserve the secret from the create response — it won't be
	// returned on subsequent reads.
	model := clientToModel(out)
	if out.Secret != nil {
		model.Secret = types.StringValue(*out.Secret)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, model)...)
}

func (r *ClientResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ClientResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	out, err := r.client.GetClient(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("OpenPanel GetClient failed", err.Error())
		return
	}

	model := clientToModel(out)
	// Secret stays as-recorded — Read never re-fetches it.
	model.Secret = state.Secret

	resp.Diagnostics.Append(resp.State.Set(ctx, model)...)
}

func (r *ClientResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state ClientResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	in := &client.SDKClient{
		Name:      plan.Name.ValueString(),
		ProjectID: plan.ProjectID.ValueString(),
		Type:      client.ClientType(plan.Type.ValueString()),
		CORS:      stringPtrOrNil(plan.CORS),
	}

	out, err := r.client.UpdateClient(ctx, plan.ID.ValueString(), in)
	if err != nil {
		resp.Diagnostics.AddError("OpenPanel UpdateClient failed", err.Error())
		return
	}

	model := clientToModel(out)
	model.Secret = state.Secret

	resp.Diagnostics.Append(resp.State.Set(ctx, model)...)
}

func (r *ClientResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ClientResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteClient(ctx, state.ID.ValueString()); err != nil {
		if client.IsNotFound(err) {
			return
		}
		resp.Diagnostics.AddError("OpenPanel DeleteClient failed", err.Error())
	}
}

func (r *ClientResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
	// Note: importing leaves the `secret` attribute null in state because
	// OpenPanel does not expose the secret on subsequent reads. Operators
	// who need the secret post-import must either store it out-of-band or
	// recreate the Client.
}

func clientToModel(c *client.SDKClient) *ClientResourceModel {
	return &ClientResourceModel{
		ID:        types.StringValue(c.ID),
		ProjectID: types.StringValue(c.ProjectID),
		Name:      types.StringValue(c.Name),
		Type:      types.StringValue(string(c.Type)),
		CORS:      stringValueOrNull(c.CORS),
		Secret:    types.StringNull(),
		CreatedAt: types.StringValue(c.CreatedAt),
		UpdatedAt: types.StringValue(c.UpdatedAt),
	}
}
