package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

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
	ReportFlowEnabled  types.Bool   `tfsdk:"report_flow_enabled"`
}

// Fields returned by GET that the create/update request does not accept.
var policyReadOnlyKeys = []string{"id", "index", "metadata"}

// firewallPolicyAPI is the API response.
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
	// Omitted on update, the API resets it to true.
	ReportFlowEnabled *bool `json:"reportFlowEnabled,omitempty"`
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
				PlanModifiers:       []planmodifier.String{equivalentFilter{}},
				MarkdownDescription: "JSON traffic filter for the source (ports, IPs, TMLs). Omit for zone-wide match.",
			},
			"destination_filter": schema.StringAttribute{
				Optional:            true,
				PlanModifiers:       []planmodifier.String{equivalentFilter{}},
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
			"report_flow_enabled": optionalComputedBool("Report matched flows in the traffic and flow view."),
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

func (r *FirewallPolicyResource) policyPath(id string) string {
	return fmt.Sprintf("/v1/sites/%s/firewall/policies/%s", r.client.SiteID(), id)
}

func (r *FirewallPolicyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan FirewallPolicyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload := map[string]any{}
	if err := applyPolicyModel(plan, payload); err != nil {
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
	if err := r.client.Get(ctx, r.policyPath(state.ID.ValueString()), &result); err != nil {
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

	// PUT replaces the whole policy, so start from the current object to keep unmanaged settings.
	var current map[string]any
	if err := r.client.Get(ctx, r.policyPath(plan.ID.ValueString()), &current); err != nil {
		resp.Diagnostics.AddError("Error reading firewall policy before update", err.Error())
		return
	}
	payload := updatePayload(current, policyReadOnlyKeys)
	if err := applyPolicyModel(plan, payload); err != nil {
		resp.Diagnostics.AddError("Error building firewall policy payload", err.Error())
		return
	}

	var result firewallPolicyAPI
	if err := r.client.Put(ctx, r.policyPath(plan.ID.ValueString()), payload, &result); err != nil {
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

	if err := r.client.Delete(ctx, r.policyPath(state.ID.ValueString())); err != nil {
		resp.Diagnostics.AddError("Error deleting firewall policy", err.Error())
	}
}

func (r *FirewallPolicyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// applyPolicyModel writes every known attribute of the model into the API payload.
func applyPolicyModel(m FirewallPolicyModel, p map[string]any) error {
	p["name"] = m.Name.ValueString()
	setBool(p, "enabled", m.Enabled)
	setBool(p, "loggingEnabled", m.LoggingEnabled)
	setBool(p, "reportFlowEnabled", m.ReportFlowEnabled)
	// Not part of the update schema, but sent only when set explicitly as before.
	if known(m.Index) {
		p["index"] = m.Index.ValueInt64()
	}

	action := child(p, "action")
	action["type"] = m.ActionType.ValueString()
	// The API rejects allowReturnTraffic for BLOCK.
	if m.ActionType.ValueString() == "ALLOW" {
		setBool(action, "allowReturnTraffic", m.AllowReturnTraffic)
	} else {
		delete(action, "allowReturnTraffic")
	}

	if err := applyPolicyEndpoint(child(p, "source"), m.SourceZoneID, m.SourceFilter, "source_filter"); err != nil {
		return err
	}
	if err := applyPolicyEndpoint(child(p, "destination"), m.DestinationZoneID, m.DestinationFilter, "destination_filter"); err != nil {
		return err
	}

	if known(m.IPVersion) && m.IPVersion.ValueString() != "" {
		scope := child(p, "ipProtocolScope")
		scope["ipVersion"] = m.IPVersion.ValueString()
		if known(m.Protocol) && m.Protocol.ValueString() != "" {
			scope["protocolFilter"] = map[string]any{
				"type":          "NAMED_PROTOCOL",
				"protocol":      map[string]any{"name": m.Protocol.ValueString()},
				"matchOpposite": false,
			}
		} else if stringAt(scope, "protocolFilter", "type").ValueString() == "NAMED_PROTOCOL" {
			delete(scope, "protocolFilter")
		}
	}

	return nil
}

// applyPolicyEndpoint sets the zone and replaces the traffic filter; a null filter matches the whole zone.
func applyPolicyEndpoint(e map[string]any, zoneID, filter types.String, attr string) error {
	e["zoneId"] = zoneID.ValueString()
	if !known(filter) || filter.ValueString() == "" {
		delete(e, "trafficFilter")
		return nil
	}
	var f map[string]any
	if err := json.Unmarshal([]byte(filter.ValueString()), &f); err != nil {
		return fmt.Errorf("invalid %s JSON: %w", attr, err)
	}
	e["trafficFilter"] = f
	return nil
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
	m.ReportFlowEnabled = types.BoolNull()
	if p.ReportFlowEnabled != nil {
		m.ReportFlowEnabled = types.BoolValue(*p.ReportFlowEnabled)
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
		f, err := filterValue(m.SourceFilter, p.Source["trafficFilter"])
		if err != nil {
			return fmt.Errorf("marshalling source trafficFilter: %w", err)
		}
		m.SourceFilter = f
	}

	// Destination zone + filter
	if p.Destination != nil {
		if zid, ok := p.Destination["zoneId"].(string); ok {
			m.DestinationZoneID = types.StringValue(zid)
		}
		f, err := filterValue(m.DestinationFilter, p.Destination["trafficFilter"])
		if err != nil {
			return fmt.Errorf("marshalling destination trafficFilter: %w", err)
		}
		m.DestinationFilter = f
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

// filterValue keeps the prior JSON when the API returns an equivalent filter, because it may reorder list items.
func filterValue(prior types.String, filter any) (types.String, error) {
	if filter == nil {
		return types.StringNull(), nil
	}
	b, err := json.Marshal(filter)
	if err != nil {
		return types.StringNull(), err
	}
	if known(prior) && equivalentJSON(prior.ValueString(), string(b)) {
		return prior, nil
	}
	return types.StringValue(string(b)), nil
}

// equivalentJSON compares two JSON documents, treating every array as an unordered list.
func equivalentJSON(a, b string) bool {
	var va, vb any
	if json.Unmarshal([]byte(a), &va) != nil || json.Unmarshal([]byte(b), &vb) != nil {
		return false
	}
	ca, _ := json.Marshal(canonicalJSON(va))
	cb, _ := json.Marshal(canonicalJSON(vb))
	return string(ca) == string(cb)
}

func canonicalJSON(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, e := range t {
			t[k] = canonicalJSON(e)
		}
		return t
	case []any:
		keys := make([]string, len(t))
		for i, e := range t {
			t[i] = canonicalJSON(e)
			raw, _ := json.Marshal(t[i])
			keys[i] = string(raw)
		}
		sort.Strings(keys)
		out := make([]any, len(keys))
		for i, k := range keys {
			_ = json.Unmarshal([]byte(k), &out[i])
		}
		return out
	}
	return v
}

// equivalentFilter keeps the state value when the configured filter only differs in list order.
type equivalentFilter struct{}

func (equivalentFilter) Description(context.Context) string {
	return "Ignores list order in traffic filter JSON."
}

func (m equivalentFilter) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (equivalentFilter) PlanModifyString(_ context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if known(req.StateValue) && known(req.PlanValue) && equivalentJSON(req.StateValue.ValueString(), req.PlanValue.ValueString()) {
		resp.PlanValue = req.StateValue
	}
}
