package resources

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/beezly/terraform-provider-unifi/internal/client"
)

// The Integration API cannot write client settings yet, so this resource uses the legacy API.

var _ resource.Resource = &ClientFixedIPResource{}
var _ resource.ResourceWithImportState = &ClientFixedIPResource{}

type ClientFixedIPResource struct {
	client *client.Client
}

func NewClientFixedIPResource() resource.Resource {
	return &ClientFixedIPResource{}
}

type ClientFixedIPModel struct {
	ID      types.String `tfsdk:"id"`
	MAC     types.String `tfsdk:"mac"`
	FixedIP types.String `tfsdk:"fixed_ip"`
}

func (r *ClientFixedIPResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_client_fixed_ip"
}

func (r *ClientFixedIPResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the DHCP fixed IP of a known client through the legacy controller API. " +
			"Destroying the resource removes the reservation; other client settings are left untouched.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Legacy client ID.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"mac": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "MAC address of the client. The controller must already know the client.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"fixed_ip": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Reserved IPv4 address.",
			},
		},
	}
}

func (r *ClientFixedIPResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// find returns the known client with the given MAC, or nil.
func (r *ClientFixedIPResource) find(ctx context.Context, mac string) (map[string]any, error) {
	var clients []map[string]any
	if err := r.client.LegacyGet(ctx, "rest/user", &clients); err != nil {
		return nil, err
	}
	return findClient(clients, mac), nil
}

func findClient(clients []map[string]any, mac string) map[string]any {
	for _, c := range clients {
		if m, _ := c["mac"].(string); strings.EqualFold(m, mac) {
			return c
		}
	}
	return nil
}

func (r *ClientFixedIPResource) write(ctx context.Context, m ClientFixedIPModel, payload map[string]any) (ClientFixedIPModel, error) {
	current, err := r.find(ctx, m.MAC.ValueString())
	if err != nil {
		return m, err
	}
	if current == nil {
		return m, fmt.Errorf("no known client with MAC %s", m.MAC.ValueString())
	}
	id, _ := current["_id"].(string)
	if err := r.client.LegacyPut(ctx, "rest/user/"+id, payload, nil); err != nil {
		return m, err
	}
	m.ID = types.StringValue(id)
	return m, nil
}

func fixedIPPayload(m ClientFixedIPModel) map[string]any {
	return map[string]any{"use_fixedip": true, "fixed_ip": m.FixedIP.ValueString()}
}

func (r *ClientFixedIPResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ClientFixedIPModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	state, err := r.write(ctx, plan, fixedIPPayload(plan))
	if err != nil {
		resp.Diagnostics.AddError("Error setting fixed IP", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *ClientFixedIPResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ClientFixedIPModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	current, err := r.find(ctx, state.MAC.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading client", err.Error())
		return
	}
	if !clientToFixedIPModel(current, &state) {
		resp.State.RemoveResource(ctx)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// clientToFixedIPModel reports false when the client or its reservation is gone.
func clientToFixedIPModel(c map[string]any, m *ClientFixedIPModel) bool {
	if c == nil || c["use_fixedip"] != true {
		return false
	}
	m.ID = stringAt(c, "_id")
	m.FixedIP = stringAt(c, "fixed_ip")
	return true
}

func (r *ClientFixedIPResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan ClientFixedIPModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	state, err := r.write(ctx, plan, fixedIPPayload(plan))
	if err != nil {
		resp.Diagnostics.AddError("Error setting fixed IP", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *ClientFixedIPResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ClientFixedIPModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if _, err := r.write(ctx, state, map[string]any{"use_fixedip": false}); err != nil {
		resp.Diagnostics.AddError("Error removing fixed IP", err.Error())
	}
}

func (r *ClientFixedIPResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("mac"), req, resp)
}
