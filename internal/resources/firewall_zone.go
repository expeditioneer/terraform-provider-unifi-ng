package resources

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/beezly/terraform-provider-unifi/internal/client"
)

var _ resource.Resource = &FirewallZoneResource{}
var _ resource.ResourceWithImportState = &FirewallZoneResource{}

type FirewallZoneResource struct {
	client *client.Client
}

func NewFirewallZoneResource() resource.Resource {
	return &FirewallZoneResource{}
}

type FirewallZoneModel struct {
	ID         types.String `tfsdk:"id"`
	Name       types.String `tfsdk:"name"`
	NetworkIDs types.List   `tfsdk:"network_ids"`
}

type firewallZoneAPI struct {
	ID         string   `json:"id,omitempty"`
	Name       string   `json:"name"`
	NetworkIDs []string `json:"networkIds,omitempty"`
}

func (r *FirewallZoneResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_firewall_zone"
}

func (r *FirewallZoneResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a UniFi firewall zone. Zones group networks for use in firewall policies.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Zone UUID.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Zone name (e.g. `Internal`, `External`, `DMZ`).",
			},
			"network_ids": schema.ListAttribute{
				ElementType:         types.StringType,
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "UUIDs of networks assigned to this zone.",
			},
		},
	}
}

func (r *FirewallZoneResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("Expected *client.Client, got %T", req.ProviderData))
		return
	}
	r.client = c
}

func (r *FirewallZoneResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan FirewallZoneModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload := zoneModelToAPI(ctx, plan)
	var result firewallZoneAPI
	if err := r.client.Post(ctx,
		fmt.Sprintf("/v1/sites/%s/firewall/zones", r.client.SiteID()),
		payload, &result); err != nil {
		resp.Diagnostics.AddError("Error creating firewall zone", err.Error())
		return
	}

	resp.Diagnostics.Append(zoneAPIToModel(ctx, &result, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *FirewallZoneResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state FirewallZoneModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var result firewallZoneAPI
	if err := r.client.Get(ctx,
		fmt.Sprintf("/v1/sites/%s/firewall/zones/%s", r.client.SiteID(), state.ID.ValueString()),
		&result); err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading firewall zone", err.Error())
		return
	}

	resp.Diagnostics.Append(zoneAPIToModel(ctx, &result, &state)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *FirewallZoneResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan FirewallZoneModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload := zoneModelToAPI(ctx, plan)
	var result firewallZoneAPI
	if err := r.client.Put(ctx,
		fmt.Sprintf("/v1/sites/%s/firewall/zones/%s", r.client.SiteID(), plan.ID.ValueString()),
		payload, &result); err != nil {
		resp.Diagnostics.AddError("Error updating firewall zone", err.Error())
		return
	}

	resp.Diagnostics.Append(zoneAPIToModel(ctx, &result, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *FirewallZoneResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state FirewallZoneModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.Delete(ctx,
		fmt.Sprintf("/v1/sites/%s/firewall/zones/%s", r.client.SiteID(), state.ID.ValueString())); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting firewall zone", err.Error())
	}
}

func (r *FirewallZoneResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func zoneModelToAPI(ctx context.Context, m FirewallZoneModel) *firewallZoneAPI {
	p := &firewallZoneAPI{Name: m.Name.ValueString()}
	if !m.NetworkIDs.IsNull() && !m.NetworkIDs.IsUnknown() {
		var ids []string
		m.NetworkIDs.ElementsAs(ctx, &ids, false)
		p.NetworkIDs = ids
	}
	return p
}

func zoneAPIToModel(ctx context.Context, p *firewallZoneAPI, m *FirewallZoneModel) diag.Diagnostics {
	m.ID = types.StringValue(p.ID)
	m.Name = types.StringValue(p.Name)
	listVal, d := types.ListValueFrom(ctx, types.StringType, p.NetworkIDs)
	m.NetworkIDs = listVal
	return d
}
