package resources

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/beezly/terraform-provider-unifi/internal/client"
)

var _ resource.Resource = &WifiBroadcastResource{}
var _ resource.ResourceWithImportState = &WifiBroadcastResource{}

type WifiBroadcastResource struct {
	client *client.Client
}

func NewWifiBroadcastResource() resource.Resource {
	return &WifiBroadcastResource{}
}

// PresharedKey is used for multi-PSK SSIDs (one passphrase per VLAN).
type PresharedKey struct {
	NetworkID  types.String `tfsdk:"network_id"`
	Passphrase types.String `tfsdk:"passphrase"`
}

type WifiBroadcastModel struct {
	ID                 types.String   `tfsdk:"id"`
	Type               types.String   `tfsdk:"type"` // STANDARD | IOT_OPTIMIZED
	Name               types.String   `tfsdk:"name"`
	Enabled            types.Bool     `tfsdk:"enabled"`
	NetworkID          types.String   `tfsdk:"network_id"`     // simple single-network SSIDs
	PresharedKeys      []PresharedKey `tfsdk:"preshared_keys"` // multi-PSK SSIDs
	SecurityType       types.String   `tfsdk:"security_type"`  // WPA3_PERSONAL | WPA2_PERSONAL | OPEN
	Passphrase         types.String   `tfsdk:"passphrase"`     // sensitive; null for multi-PSK
	HideName           types.Bool     `tfsdk:"hide_name"`
	ClientIsolation    types.Bool     `tfsdk:"client_isolation"`
	BandSteering       types.Bool     `tfsdk:"band_steering"` // STANDARD only
	FastRoaming        types.Bool     `tfsdk:"fast_roaming"`  // STANDARD only
	MulticastToUnicast types.Bool     `tfsdk:"multicast_to_unicast"`
	FrequenciesGHz     types.Set      `tfsdk:"frequencies_ghz"` // STANDARD only
}

type wifiBroadcastAPI struct {
	ID                                  string                 `json:"id,omitempty"`
	Type                                string                 `json:"type,omitempty"`
	Name                                string                 `json:"name"`
	Enabled                             *bool                  `json:"enabled,omitempty"`
	Network                             map[string]interface{} `json:"network,omitempty"`
	SecurityConfiguration               map[string]interface{} `json:"securityConfiguration,omitempty"`
	HideName                            bool                   `json:"hideName"`
	ClientIsolationEnabled              bool                   `json:"clientIsolationEnabled"`
	BandSteeringEnabled                 *bool                  `json:"bandSteeringEnabled,omitempty"`
	FastRoamingEnabled                  bool                   `json:"fastRoamingEnabled,omitempty"` // deprecated top-level
	MulticastToUnicastConversionEnabled bool                   `json:"multicastToUnicastConversionEnabled"`
	BroadcastingFrequenciesGHz          []float64              `json:"broadcastingFrequenciesGHz,omitempty"`
	ArpProxyEnabled                     bool                   `json:"arpProxyEnabled"`
	UapsdEnabled                        bool                   `json:"uapsdEnabled"`
	BssTransitionEnabled                bool                   `json:"bssTransitionEnabled"`
	AdvertiseDeviceName                 bool                   `json:"advertiseDeviceName"`
}

func (r *WifiBroadcastResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_wifi_broadcast"
}

func (r *WifiBroadcastResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a UniFi WiFi broadcast (SSID).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"type": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Broadcast type: `STANDARD` (default) or `IOT_OPTIMIZED` (2.4 GHz only, no band steering).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "SSID name.",
			},
			"enabled": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(true),
			},
			"network_id": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "UUID of the network (VLAN) this SSID maps to. Use `preshared_keys` instead for multi-PSK SSIDs.",
			},
			"preshared_keys": schema.ListNestedAttribute{
				Optional:            true,
				MarkdownDescription: "Per-network passphrases for multi-PSK SSIDs. When set, `network_id` and `passphrase` are ignored.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"network_id": schema.StringAttribute{
							Required: true,
						},
						"passphrase": schema.StringAttribute{
							Required:  true,
							Sensitive: true,
						},
					},
				},
			},
			"security_type": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Security type: `WPA3_PERSONAL`, `WPA2_PERSONAL`, `WPA2_WPA3_PERSONAL`, or `OPEN`.",
			},
			"passphrase": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: "WiFi passphrase. Required unless `security_type = OPEN`.",
			},
			"hide_name": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(false),
				MarkdownDescription: "Hide SSID from broadcast.",
			},
			"client_isolation": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(false),
				MarkdownDescription: "Isolate clients from each other.",
			},
			"band_steering": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
				MarkdownDescription: "Steer capable clients to 5/6 GHz. Only applies to `STANDARD` type.",
			},
			"fast_roaming": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
				MarkdownDescription: "Enable 802.11r fast BSS transition. Only applies to `STANDARD` type.",
			},
			"multicast_to_unicast": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(false),
				MarkdownDescription: "Convert multicast traffic to unicast.",
			},
			"frequencies_ghz": schema.SetAttribute{
				ElementType:         types.StringType,
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Set{setplanmodifier.UseStateForUnknown()},
				MarkdownDescription: "Broadcasting frequencies: `2.4`, `5`, `6`.",
			},
		},
	}
}

func (r *WifiBroadcastResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *WifiBroadcastResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan WifiBroadcastModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	payload, d := wifiModelToAPI(ctx, plan)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	var result wifiBroadcastAPI
	if err := r.client.Post(ctx, fmt.Sprintf("/v1/sites/%s/wifi/broadcasts", r.client.SiteID()), payload, &result); err != nil {
		resp.Diagnostics.AddError("Error creating wifi broadcast", err.Error())
		return
	}
	resp.Diagnostics.Append(wifiAPIToModel(ctx, &result, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *WifiBroadcastResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state WifiBroadcastModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var result wifiBroadcastAPI
	if err := r.client.Get(ctx, fmt.Sprintf("/v1/sites/%s/wifi/broadcasts/%s", r.client.SiteID(), state.ID.ValueString()), &result); err != nil {
		resp.Diagnostics.AddError("Error reading wifi broadcast", err.Error())
		return
	}
	// If the API didn't return a passphrase, preserve the one from state
	// (some API versions redact it; others return it)
	if result.SecurityConfiguration != nil {
		if _, hasPass := result.SecurityConfiguration["passphrase"]; !hasPass {
			result.SecurityConfiguration["passphrase"] = state.Passphrase.ValueString()
		}
	}
	resp.Diagnostics.Append(wifiAPIToModel(ctx, &result, &state)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *WifiBroadcastResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan WifiBroadcastModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	payload, d := wifiModelToAPI(ctx, plan)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	var result wifiBroadcastAPI
	if err := r.client.Put(ctx, fmt.Sprintf("/v1/sites/%s/wifi/broadcasts/%s", r.client.SiteID(), plan.ID.ValueString()), payload, &result); err != nil {
		resp.Diagnostics.AddError("Error updating wifi broadcast", err.Error())
		return
	}
	resp.Diagnostics.Append(wifiAPIToModel(ctx, &result, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *WifiBroadcastResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state WifiBroadcastModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.Delete(ctx, fmt.Sprintf("/v1/sites/%s/wifi/broadcasts/%s", r.client.SiteID(), state.ID.ValueString())); err != nil {
		resp.Diagnostics.AddError("Error deleting wifi broadcast", err.Error())
	}
}

func (r *WifiBroadcastResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func wifiModelToAPI(ctx context.Context, m WifiBroadcastModel) (*wifiBroadcastAPI, diag.Diagnostics) {
	enabled := m.Enabled.ValueBool()
	broadcastType := "STANDARD"
	if !m.Type.IsNull() && !m.Type.IsUnknown() && m.Type.ValueString() != "" {
		broadcastType = m.Type.ValueString()
	}
	p := &wifiBroadcastAPI{
		Type:    broadcastType,
		Name:    m.Name.ValueString(),
		Enabled: &enabled,
		Network: map[string]interface{}{
			"type":      "SPECIFIC",
			"networkId": m.NetworkID.ValueString(),
		},
		SecurityConfiguration: map[string]interface{}{
			"type": m.SecurityType.ValueString(),
		},
		HideName:                            m.HideName.ValueBool(),
		ClientIsolationEnabled:              m.ClientIsolation.ValueBool(),
		MulticastToUnicastConversionEnabled: m.MulticastToUnicast.ValueBool(),
	}

	// Band steering — only include when true (API rejects false with single band)
	if !m.BandSteering.IsNull() && !m.BandSteering.IsUnknown() && m.BandSteering.ValueBool() {
		bs := true
		p.BandSteeringEnabled = &bs
	}

	// WPA3-specific security configuration defaults
	secType := m.SecurityType.ValueString()
	if secType == "WPA2_WPA3_PERSONAL" || secType == "WPA3_PERSONAL" {
		pmfMode := "OPTIONAL"
		if secType == "WPA3_PERSONAL" {
			pmfMode = "REQUIRED"
		}
		p.SecurityConfiguration["pmfMode"] = pmfMode
		p.SecurityConfiguration["saeConfiguration"] = map[string]interface{}{
			"anticloggingThresholdSeconds": 5,
			"syncTimeSeconds":              5,
		}
		p.SecurityConfiguration["wpa3FastRoamingEnabled"] = false
	}

	if len(m.PresharedKeys) > 0 {
		// Multi-PSK: one passphrase per network
		var keys []map[string]interface{}
		for _, k := range m.PresharedKeys {
			keys = append(keys, map[string]interface{}{
				"network":    map[string]interface{}{"type": "SPECIFIC", "networkId": k.NetworkID.ValueString()},
				"passphrase": k.Passphrase.ValueString(),
			})
		}
		p.SecurityConfiguration["presharedKeys"] = keys
		p.Network = nil // no top-level network for multi-PSK
	} else if !m.Passphrase.IsNull() && !m.Passphrase.IsUnknown() && m.Passphrase.ValueString() != "" {
		p.SecurityConfiguration["passphrase"] = m.Passphrase.ValueString()
	}
	if !m.FastRoaming.IsNull() && !m.FastRoaming.IsUnknown() {
		p.SecurityConfiguration["fastRoamingEnabled"] = m.FastRoaming.ValueBool()
	}

	// Frequencies
	if !m.FrequenciesGHz.IsNull() && !m.FrequenciesGHz.IsUnknown() {
		var freqStrs []string
		m.FrequenciesGHz.ElementsAs(ctx, &freqStrs, false)
		var freqs []float64
		for _, s := range freqStrs {
			var f float64
			fmt.Sscanf(s, "%f", &f)
			freqs = append(freqs, f)
		}
		p.BroadcastingFrequenciesGHz = freqs
	}

	return p, nil
}

func wifiAPIToModel(ctx context.Context, p *wifiBroadcastAPI, m *WifiBroadcastModel) diag.Diagnostics {
	var diags diag.Diagnostics
	m.ID = types.StringValue(p.ID)
	m.Type = types.StringValue(p.Type)
	m.Name = types.StringValue(p.Name)
	if p.Enabled != nil {
		m.Enabled = types.BoolValue(*p.Enabled)
	}
	m.HideName = types.BoolValue(p.HideName)
	m.ClientIsolation = types.BoolValue(p.ClientIsolationEnabled)
	m.MulticastToUnicast = types.BoolValue(p.MulticastToUnicastConversionEnabled)

	// STANDARD-only fields — null for IOT_OPTIMIZED
	if p.Type == "IOT_OPTIMIZED" {
		m.BandSteering = types.BoolNull()
		m.FastRoaming = types.BoolNull()
		setVal, _ := types.SetValueFrom(ctx, types.StringType, []string{})
		m.FrequenciesGHz = setVal
	} else {
		if p.BandSteeringEnabled != nil {
			m.BandSteering = types.BoolValue(*p.BandSteeringEnabled)
		}
	}

	if p.Network != nil {
		if nid, ok := p.Network["networkId"].(string); ok {
			m.NetworkID = types.StringValue(nid)
		}
	} else {
		m.NetworkID = types.StringNull()
	}

	if p.SecurityConfiguration != nil {
		if t, ok := p.SecurityConfiguration["type"].(string); ok {
			m.SecurityType = types.StringValue(t)
		}
		if fr, ok := p.SecurityConfiguration["fastRoamingEnabled"].(bool); ok {
			m.FastRoaming = types.BoolValue(fr)
		}

		// Multi-PSK: presharedKeys array
		if keys, ok := p.SecurityConfiguration["presharedKeys"].([]interface{}); ok && len(keys) > 0 {
			m.PresharedKeys = nil
			m.Passphrase = types.StringNull()
			for _, k := range keys {
				km, ok := k.(map[string]interface{})
				if !ok {
					continue
				}
				net, _ := km["network"].(map[string]interface{})
				nid, _ := net["networkId"].(string)
				pass, _ := km["passphrase"].(string)
				m.PresharedKeys = append(m.PresharedKeys, PresharedKey{
					NetworkID:  types.StringValue(nid),
					Passphrase: types.StringValue(pass),
				})
			}
		} else {
			// Single PSK — nil (not empty slice) so Terraform treats as null
			m.PresharedKeys = nil
			if pass, ok := p.SecurityConfiguration["passphrase"].(string); ok && pass != "" {
				m.Passphrase = types.StringValue(pass)
			}
		}
	}

	// Frequencies — store as strings since 2.4 can't be a TF number attribute name
	var freqStrs []string
	for _, f := range p.BroadcastingFrequenciesGHz {
		if f == 2.4 {
			freqStrs = append(freqStrs, "2.4")
		} else {
			freqStrs = append(freqStrs, fmt.Sprintf("%.0f", f))
		}
	}
	setVal, d := types.SetValueFrom(ctx, types.StringType, freqStrs)
	diags.Append(d...)
	m.FrequenciesGHz = setVal

	return diags
}
