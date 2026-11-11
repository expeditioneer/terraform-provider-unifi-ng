package resources

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/beezly/terraform-provider-unifi/internal/client"
)

var _ resource.Resource = &FirewallPolicyResource{}
var _ resource.ResourceWithImportState = &FirewallPolicyResource{}

type FirewallPolicyResource struct {
	client *client.Client
}

func NewFirewallPolicyResource() resource.Resource {
	return &FirewallPolicyResource{}
}

// FirewallPolicyModel is the Terraform schema model.
type FirewallPolicyModel struct {
	ID                 types.String `tfsdk:"id"`
	Name               types.String `tfsdk:"name"`
	Enabled            types.Bool   `tfsdk:"enabled"`
	Index              types.Int64  `tfsdk:"index"`
	ActionType         types.String `tfsdk:"action_type"` // ALLOW | BLOCK
	AllowReturnTraffic types.Bool   `tfsdk:"allow_return_traffic"`
	SourceZoneID       types.String `tfsdk:"source_zone_id"`
	DestinationZoneID  types.String `tfsdk:"destination_zone_id"`
	SourceFilter       types.String `tfsdk:"source_filter"`      // JSON blob, optional
	DestinationFilter  types.String `tfsdk:"destination_filter"` // JSON blob, optional
	IPVersion          types.String `tfsdk:"ip_version"`         // IPV4 | IPV6 | IPV4_AND_IPV6
	Protocol           types.String `tfsdk:"protocol"`           // TCP | UDP | etc., optional
	LoggingEnabled     types.Bool   `tfsdk:"logging_enabled"`
}

// firewallPolicyAPI is the raw API payload (used for both send and receive).
type firewallPolicyAPI struct {
	ID              string                 `json:"id,omitempty"`
	Name            string                 `json:"name"`
	Enabled         *bool                  `json:"enabled,omitempty"`
	Index           *int64                 `json:"index,omitempty"`
	Action          map[string]interface{} `json:"action,omitempty"`
	Source          map[string]interface{} `json:"source,omitempty"`
	Destination     map[string]interface{} `json:"destination,omitempty"`
	IPProtocolScope map[string]interface{} `json:"ipProtocolScope,omitempty"`
	LoggingEnabled  bool                   `json:"loggingEnabled"`
}

func (r *FirewallPolicyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_firewall_policy"
}

func (r *FirewallPolicyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a UniFi firewall policy rule.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Policy UUID.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Policy name.",
			},
			"enabled": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
				MarkdownDescription: "Whether the policy is active.",
			},
			"index": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Ordering index. Lower = higher priority.",
			},
			"action_type": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Action to take: `ALLOW` or `BLOCK`.",
			},
			"allow_return_traffic": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "Allow return (stateful) traffic. Only applies when `action_type = ALLOW`.",
			},
			"source_zone_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "UUID of the source firewall zone.",
			},
			"destination_zone_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "UUID of the destination firewall zone.",
			},
			"source_filter": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "JSON traffic filter for the source (ports, IPs, TMLs). Omit for zone-wide match.",
			},
			"destination_filter": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "JSON traffic filter for the destination (ports, IPs, TMLs). Omit for zone-wide match.",
			},
			"ip_version": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "IP version scope: `IPV4`, `IPV6`, or `IPV4_AND_IPV6`.",
			},
			"protocol": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Protocol filter: `TCP`, `UDP`, `ICMP`, etc. Omit for any.",
			},
			"logging_enabled": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "Log matched traffic.",
			},
		},
	}
}

func (r *FirewallPolicyResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *FirewallPolicyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan FirewallPolicyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload, err := policyModelToAPI(plan)
	if err != nil {
		resp.Diagnostics.AddError("Error building firewall policy payload", err.Error())
		return
	}

	var result firewallPolicyAPI
	if err := r.client.Post(ctx,
		fmt.Sprintf("/v1/sites/%s/firewall/policies", r.client.SiteID()),
		payload, &result); err != nil {
		resp.Diagnostics.AddError("Error creating firewall policy", err.Error())
		return
	}

	if err := policyAPIToModel(&result, &plan); err != nil {
		resp.Diagnostics.AddError("Error reading firewall policy response", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *FirewallPolicyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state FirewallPolicyModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var result firewallPolicyAPI
	if err := r.client.Get(ctx,
		fmt.Sprintf("/v1/sites/%s/firewall/policies/%s", r.client.SiteID(), state.ID.ValueString()),
		&result); err != nil {
		resp.Diagnostics.AddError("Error reading firewall policy", err.Error())
		return
	}

	if err := policyAPIToModel(&result, &state); err != nil {
		resp.Diagnostics.AddError("Error parsing firewall policy response", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *FirewallPolicyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan FirewallPolicyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload, err := policyModelToAPI(plan)
	if err != nil {
		resp.Diagnostics.AddError("Error building firewall policy payload", err.Error())
		return
	}

	var result firewallPolicyAPI
	if err := r.client.Put(ctx,
		fmt.Sprintf("/v1/sites/%s/firewall/policies/%s", r.client.SiteID(), plan.ID.ValueString()),
		payload, &result); err != nil {
		resp.Diagnostics.AddError("Error updating firewall policy", err.Error())
		return
	}

	if err := policyAPIToModel(&result, &plan); err != nil {
		resp.Diagnostics.AddError("Error reading firewall policy response", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *FirewallPolicyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state FirewallPolicyModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.Delete(ctx,
		fmt.Sprintf("/v1/sites/%s/firewall/policies/%s", r.client.SiteID(), state.ID.ValueString())); err != nil {
		resp.Diagnostics.AddError("Error deleting firewall policy", err.Error())
	}
}

func (r *FirewallPolicyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// policyModelToAPI converts the Terraform model to the API payload.
func policyModelToAPI(m FirewallPolicyModel) (*firewallPolicyAPI, error) {
	enabled := m.Enabled.ValueBool()
	p := &firewallPolicyAPI{
		Name:           m.Name.ValueString(),
		Enabled:        &enabled,
		LoggingEnabled: m.LoggingEnabled.ValueBool(),
		Action: map[string]interface{}{
			"type": m.ActionType.ValueString(),
		},
	}

	// Only include allowReturnTraffic for ALLOW actions (API rejects it for BLOCK)
	if m.ActionType.ValueString() == "ALLOW" {
		p.Action["allowReturnTraffic"] = m.AllowReturnTraffic.ValueBool()
	}

	if !m.Index.IsNull() && !m.Index.IsUnknown() {
		idx := m.Index.ValueInt64()
		p.Index = &idx
	}

	// Source
	source := map[string]interface{}{"zoneId": m.SourceZoneID.ValueString()}
	if !m.SourceFilter.IsNull() && !m.SourceFilter.IsUnknown() && m.SourceFilter.ValueString() != "" {
		var filter map[string]interface{}
		if err := json.Unmarshal([]byte(m.SourceFilter.ValueString()), &filter); err != nil {
			return nil, fmt.Errorf("invalid source_filter JSON: %w", err)
		}
		source["trafficFilter"] = filter
	}
	p.Source = source

	// Destination
	dest := map[string]interface{}{"zoneId": m.DestinationZoneID.ValueString()}
	if !m.DestinationFilter.IsNull() && !m.DestinationFilter.IsUnknown() && m.DestinationFilter.ValueString() != "" {
		var filter map[string]interface{}
		if err := json.Unmarshal([]byte(m.DestinationFilter.ValueString()), &filter); err != nil {
			return nil, fmt.Errorf("invalid destination_filter JSON: %w", err)
		}
		dest["trafficFilter"] = filter
	}
	p.Destination = dest

	// IP protocol scope
	if !m.IPVersion.IsNull() && !m.IPVersion.IsUnknown() && m.IPVersion.ValueString() != "" {
		scope := map[string]interface{}{"ipVersion": m.IPVersion.ValueString()}
		if !m.Protocol.IsNull() && !m.Protocol.IsUnknown() && m.Protocol.ValueString() != "" {
			scope["protocolFilter"] = map[string]interface{}{
				"type": "NAMED_PROTOCOL",
				"protocol": map[string]interface{}{
					"name": m.Protocol.ValueString(),
				},
				"matchOpposite": false,
			}
		}
		p.IPProtocolScope = scope
	}

	return p, nil
}

// policyAPIToModel maps the API response back to the Terraform model.
func policyAPIToModel(p *firewallPolicyAPI, m *FirewallPolicyModel) error {
	m.ID = types.StringValue(p.ID)
	m.Name = types.StringValue(p.Name)
	m.LoggingEnabled = types.BoolValue(p.LoggingEnabled)

	if p.Enabled != nil {
		m.Enabled = types.BoolValue(*p.Enabled)
	} else {
		m.Enabled = types.BoolValue(true)
	}
	if p.Index != nil {
		m.Index = types.Int64Value(*p.Index)
	}

	// Action
	if p.Action != nil {
		if t, ok := p.Action["type"].(string); ok {
			m.ActionType = types.StringValue(t)
		}
		if art, ok := p.Action["allowReturnTraffic"].(bool); ok {
			m.AllowReturnTraffic = types.BoolValue(art)
		} else {
			m.AllowReturnTraffic = types.BoolValue(false)
		}
	}

	// Source zone + filter
	if p.Source != nil {
		if zid, ok := p.Source["zoneId"].(string); ok {
			m.SourceZoneID = types.StringValue(zid)
		}
		if tf, ok := p.Source["trafficFilter"]; ok && tf != nil {
			b, err := json.Marshal(tf)
			if err != nil {
				return fmt.Errorf("marshalling source trafficFilter: %w", err)
			}
			m.SourceFilter = types.StringValue(string(b))
		} else {
			m.SourceFilter = types.StringNull()
		}
	}

	// Destination zone + filter
	if p.Destination != nil {
		if zid, ok := p.Destination["zoneId"].(string); ok {
			m.DestinationZoneID = types.StringValue(zid)
		}
		if tf, ok := p.Destination["trafficFilter"]; ok && tf != nil {
			b, err := json.Marshal(tf)
			if err != nil {
				return fmt.Errorf("marshalling destination trafficFilter: %w", err)
			}
			m.DestinationFilter = types.StringValue(string(b))
		} else {
			m.DestinationFilter = types.StringNull()
		}
	}

	// IP protocol scope
	if p.IPProtocolScope != nil {
		if ver, ok := p.IPProtocolScope["ipVersion"].(string); ok {
			m.IPVersion = types.StringValue(ver)
		}
		if pf, ok := p.IPProtocolScope["protocolFilter"].(map[string]interface{}); ok {
			if proto, ok := pf["protocol"].(map[string]interface{}); ok {
				if name, ok := proto["name"].(string); ok {
					m.Protocol = types.StringValue(name)
				}
			}
		}
	}
	if m.IPVersion.IsNull() || m.IPVersion.IsUnknown() {
		m.IPVersion = types.StringNull()
	}
	if m.Protocol.IsNull() || m.Protocol.IsUnknown() {
		m.Protocol = types.StringNull()
	}

	return nil
}
