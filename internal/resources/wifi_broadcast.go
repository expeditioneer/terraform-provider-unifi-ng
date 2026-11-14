package resources

import (
	"context"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
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
	ID                  types.String   `tfsdk:"id"`
	Type                types.String   `tfsdk:"type"` // STANDARD | IOT_OPTIMIZED
	Name                types.String   `tfsdk:"name"`
	Enabled             types.Bool     `tfsdk:"enabled"`
	NetworkID           types.String   `tfsdk:"network_id"`     // simple single-network SSIDs
	PresharedKeys       []PresharedKey `tfsdk:"preshared_keys"` // multi-PSK SSIDs
	SecurityType        types.String   `tfsdk:"security_type"`  // WPA3_PERSONAL | WPA2_PERSONAL | OPEN
	Passphrase          types.String   `tfsdk:"passphrase"`     // sensitive; null for multi-PSK
	HideName            types.Bool     `tfsdk:"hide_name"`
	ClientIsolation     types.Bool     `tfsdk:"client_isolation"`
	BandSteering        types.Bool     `tfsdk:"band_steering"` // STANDARD only
	FastRoaming         types.Bool     `tfsdk:"fast_roaming"`  // STANDARD only
	MulticastToUnicast  types.Bool     `tfsdk:"multicast_to_unicast"`
	FrequenciesGHz      types.Set      `tfsdk:"frequencies_ghz"` // STANDARD only
	AdvertiseDeviceName types.Bool     `tfsdk:"advertise_device_name"`
	ArpProxy            types.Bool     `tfsdk:"arp_proxy"`
	BroadcastDeviceIDs  types.Set      `tfsdk:"broadcasting_device_ids"`
	BroadcastTagIDs     types.Set      `tfsdk:"broadcasting_device_tag_ids"`
	BssTransition       types.Bool     `tfsdk:"bss_transition"`
	PmfMode             types.String   `tfsdk:"pmf_mode"`
	Uapsd               types.Bool     `tfsdk:"uapsd"`
	Wpa3FastRoaming     types.Bool     `tfsdk:"wpa3_fast_roaming"`
}

// Fields returned by GET that the create/update request does not accept.
var wifiReadOnlyKeys = []string{"id", "metadata"}

func (r *WifiBroadcastResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_wifi_broadcast"
}

func (r *WifiBroadcastResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a UniFi WiFi broadcast (SSID). Updates only change the attributes managed here; all other settings of the SSID are preserved.",
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
			"band_steering": optionalComputedBool("Steer capable clients to 5/6 GHz. Only applies to `STANDARD` type."),
			"fast_roaming":  optionalComputedBool("Enable 802.11r fast BSS transition. Only applies to `STANDARD` type."),
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
			"advertise_device_name": optionalComputedBool("Advertise the access point name in beacons."),
			"arp_proxy":             optionalComputedBool("Answer ARP requests on behalf of clients."),
			"broadcasting_device_ids": broadcastFilterAttribute(
				"Access point IDs that broadcast this SSID. Empty means all access points. Conflicts with `broadcasting_device_tag_ids`."),
			"broadcasting_device_tag_ids": broadcastFilterAttribute(
				"Device tag IDs whose access points broadcast this SSID. Empty means all access points. Conflicts with `broadcasting_device_ids`."),
			"bss_transition":    optionalComputedBool("Enable 802.11v BSS transition management."),
			"pmf_mode":          optionalComputedString("Protected management frames: `OPTIONAL` or `REQUIRED`. Only applies to WPA3 security types; defaults to `OPTIONAL` for `WPA2_WPA3_PERSONAL` and `REQUIRED` for `WPA3_PERSONAL`."),
			"uapsd":             optionalComputedBool("Enable unscheduled automatic power save delivery."),
			"wpa3_fast_roaming": optionalComputedBool("Enable fast roaming for WPA3. Only applies to WPA3 security types."),
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

func (r *WifiBroadcastResource) wifiPath(id string) string {
	return fmt.Sprintf("/v1/sites/%s/wifi/broadcasts/%s", r.client.SiteID(), id)
}

// newWifiPayload holds the fields the API requires on create that are not managed as attributes.
func newWifiPayload() map[string]any {
	return map[string]any{
		"advertiseDeviceName":   false,
		"arpProxyEnabled":       false,
		"bssTransitionEnabled":  false,
		"channel2gLockedTo6":    false,
		"dtimPeriod2gLockedTo3": false,
		"uapsdEnabled":          false,
	}
}

func (r *WifiBroadcastResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan WifiBroadcastModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	payload := newWifiPayload()
	resp.Diagnostics.Append(applyWifiModel(ctx, plan, payload)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var result map[string]any
	if err := r.client.Post(ctx, fmt.Sprintf("/v1/sites/%s/wifi/broadcasts", r.client.SiteID()), payload, &result); err != nil {
		resp.Diagnostics.AddError("Error creating wifi broadcast", err.Error())
		return
	}
	resp.Diagnostics.Append(wifiToModel(ctx, result, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *WifiBroadcastResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state WifiBroadcastModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var result map[string]any
	if err := r.client.Get(ctx, r.wifiPath(state.ID.ValueString()), &result); err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading wifi broadcast", err.Error())
		return
	}
	// Some API versions redact the passphrase.
	if sec, ok := result["securityConfiguration"].(map[string]any); ok {
		if _, hasPass := sec["passphrase"]; !hasPass {
			sec["passphrase"] = state.Passphrase.ValueString()
		}
	}
	resp.Diagnostics.Append(wifiToModel(ctx, result, &state)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *WifiBroadcastResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan WifiBroadcastModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// PUT replaces the whole SSID, so start from the current object to keep unmanaged settings.
	var current map[string]any
	if err := r.client.Get(ctx, r.wifiPath(plan.ID.ValueString()), &current); err != nil {
		resp.Diagnostics.AddError("Error reading wifi broadcast before update", err.Error())
		return
	}
	payload := updatePayload(current, wifiReadOnlyKeys)
	resp.Diagnostics.Append(applyWifiModel(ctx, plan, payload)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var result map[string]any
	if err := r.client.Put(ctx, r.wifiPath(plan.ID.ValueString()), payload, &result); err != nil {
		resp.Diagnostics.AddError("Error updating wifi broadcast", err.Error())
		return
	}
	resp.Diagnostics.Append(wifiToModel(ctx, result, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *WifiBroadcastResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state WifiBroadcastModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.Delete(ctx, r.wifiPath(state.ID.ValueString())); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting wifi broadcast", err.Error())
	}
}

func (r *WifiBroadcastResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func broadcastFilterAttribute(description string) schema.SetAttribute {
	return schema.SetAttribute{
		ElementType:         types.StringType,
		Optional:            true,
		Computed:            true,
		PlanModifiers:       []planmodifier.Set{setplanmodifier.UseStateForUnknown()},
		MarkdownDescription: description,
	}
}

func setBool(p map[string]any, key string, v types.Bool) {
	if known(v) {
		p[key] = v.ValueBool()
	}
}

// applyWifiModel writes every known attribute of the model into the API payload.
func applyWifiModel(ctx context.Context, m WifiBroadcastModel, p map[string]any) diag.Diagnostics {
	var diags diag.Diagnostics

	p["name"] = m.Name.ValueString()
	if known(m.Type) && m.Type.ValueString() != "" {
		p["type"] = m.Type.ValueString()
	} else if _, ok := p["type"]; !ok {
		p["type"] = "STANDARD"
	}
	setBool(p, "enabled", m.Enabled)
	setBool(p, "hideName", m.HideName)
	setBool(p, "clientIsolationEnabled", m.ClientIsolation)
	setBool(p, "multicastToUnicastConversionEnabled", m.MulticastToUnicast)
	setBool(p, "advertiseDeviceName", m.AdvertiseDeviceName)
	setBool(p, "arpProxyEnabled", m.ArpProxy)
	setBool(p, "bssTransitionEnabled", m.BssTransition)
	setBool(p, "uapsdEnabled", m.Uapsd)

	diags.Append(applyBroadcastFilter(ctx, m, p)...)

	// The API rejects false with a single band, where it omits the field.
	if known(m.BandSteering) {
		if _, present := p["bandSteeringEnabled"]; present || m.BandSteering.ValueBool() {
			p["bandSteeringEnabled"] = m.BandSteering.ValueBool()
		}
	}

	if known(m.FrequenciesGHz) {
		var freqStrs []string
		diags.Append(m.FrequenciesGHz.ElementsAs(ctx, &freqStrs, false)...)
		freqs := []float64{}
		for _, s := range freqStrs {
			f, err := strconv.ParseFloat(s, 64)
			if err != nil {
				diags.AddError("Invalid frequencies_ghz", err.Error())
				return diags
			}
			freqs = append(freqs, f)
		}
		if len(freqs) > 0 && !sameFrequencies(p["broadcastingFrequenciesGHz"], freqs) {
			p["broadcastingFrequenciesGHz"] = freqs
		}
	}

	secType := m.SecurityType.ValueString()
	sec, _ := p["securityConfiguration"].(map[string]any)
	if sec == nil || sec["type"] != secType {
		sec = map[string]any{"type": secType}
		p["securityConfiguration"] = sec
	}
	if secType == "WPA2_WPA3_PERSONAL" || secType == "WPA3_PERSONAL" {
		if _, ok := sec["pmfMode"]; !ok {
			sec["pmfMode"] = "OPTIONAL"
			if secType == "WPA3_PERSONAL" {
				sec["pmfMode"] = "REQUIRED"
			}
		}
		if _, ok := sec["saeConfiguration"]; !ok {
			sec["saeConfiguration"] = map[string]any{
				"anticloggingThresholdSeconds": 5,
				"syncTimeSeconds":              5,
			}
		}
		if _, ok := sec["wpa3FastRoamingEnabled"]; !ok {
			sec["wpa3FastRoamingEnabled"] = false
		}
		if known(m.PmfMode) {
			sec["pmfMode"] = m.PmfMode.ValueString()
		}
		setBool(sec, "wpa3FastRoamingEnabled", m.Wpa3FastRoaming)
	}
	setBool(sec, "fastRoamingEnabled", m.FastRoaming)

	if len(m.PresharedKeys) > 0 {
		keys := []map[string]any{}
		for _, k := range m.PresharedKeys {
			keys = append(keys, map[string]any{
				"network":    map[string]any{"type": "SPECIFIC", "networkId": k.NetworkID.ValueString()},
				"passphrase": k.Passphrase.ValueString(),
			})
		}
		sec["presharedKeys"] = keys
		delete(sec, "passphrase")
		delete(p, "network")
	} else {
		delete(sec, "presharedKeys")
		if known(m.Passphrase) && m.Passphrase.ValueString() != "" {
			sec["passphrase"] = m.Passphrase.ValueString()
		}
		if known(m.NetworkID) {
			p["network"] = map[string]any{"type": "SPECIFIC", "networkId": m.NetworkID.ValueString()}
		}
	}

	return diags
}

// applyBroadcastFilter maps both ID sets onto the single broadcastingDeviceFilter; null means all access points.
func applyBroadcastFilter(ctx context.Context, m WifiBroadcastModel, p map[string]any) diag.Diagnostics {
	var diags diag.Diagnostics
	filters := []struct {
		typ, key string
		ids      []string
	}{{typ: "DEVICES", key: "deviceIds"}, {typ: "DEVICE_TAGS", key: "deviceTagIds"}}
	values := []types.Set{m.BroadcastDeviceIDs, m.BroadcastTagIDs}
	configured := 0
	for i := range filters {
		if known(values[i]) {
			diags.Append(values[i].ElementsAs(ctx, &filters[i].ids, false)...)
		}
		if len(filters[i].ids) > 0 {
			configured++
		}
	}
	if configured > 1 {
		diags.AddError("Conflicting broadcast filters", "Set only one of broadcasting_device_ids and broadcasting_device_tag_ids.")
	}
	if diags.HasError() {
		return diags
	}
	for i, f := range filters {
		current, _ := p["broadcastingDeviceFilter"].(map[string]any)
		switch {
		case len(f.ids) > 0:
			p["broadcastingDeviceFilter"] = map[string]any{"type": f.typ, f.key: f.ids}
		case known(values[i]) && current != nil && current["type"] == f.typ:
			p["broadcastingDeviceFilter"] = nil
		}
	}
	return diags
}

// broadcastFilterIDs returns the IDs of a filter of the given type, empty otherwise.
func broadcastFilterIDs(ctx context.Context, p map[string]any, typ, key string) (types.Set, diag.Diagnostics) {
	ids := []string{}
	if f, ok := p["broadcastingDeviceFilter"].(map[string]any); ok && f["type"] == typ {
		list, _ := f[key].([]any)
		for _, id := range list {
			if s, ok := id.(string); ok {
				ids = append(ids, s)
			}
		}
	}
	return types.SetValueFrom(ctx, types.StringType, ids)
}

// sameFrequencies ignores order, so an unchanged set keeps the API's ordering.
func sameFrequencies(current any, freqs []float64) bool {
	list, ok := current.([]any)
	if !ok || len(list) != len(freqs) {
		return false
	}
	seen := map[float64]bool{}
	for _, f := range list {
		n, ok := f.(float64)
		if !ok {
			return false
		}
		seen[n] = true
	}
	for _, f := range freqs {
		if !seen[f] {
			return false
		}
	}
	return true
}

func boolOrFalse(p map[string]any, keys ...string) types.Bool {
	if v := boolAt(p, keys...); !v.IsNull() {
		return v
	}
	return types.BoolValue(false)
}

// wifiToModel maps an API WiFi broadcast object into the Terraform model.
func wifiToModel(ctx context.Context, p map[string]any, m *WifiBroadcastModel) diag.Diagnostics {
	var diags diag.Diagnostics

	m.ID = stringAt(p, "id")
	m.Type = stringAt(p, "type")
	m.Name = stringAt(p, "name")
	if v := boolAt(p, "enabled"); !v.IsNull() {
		m.Enabled = v
	}
	m.HideName = boolOrFalse(p, "hideName")
	m.ClientIsolation = boolOrFalse(p, "clientIsolationEnabled")
	m.MulticastToUnicast = boolOrFalse(p, "multicastToUnicastConversionEnabled")
	m.AdvertiseDeviceName = boolAt(p, "advertiseDeviceName")
	m.ArpProxy = boolAt(p, "arpProxyEnabled")
	m.BssTransition = boolAt(p, "bssTransitionEnabled")
	m.Uapsd = boolAt(p, "uapsdEnabled")

	var d diag.Diagnostics
	m.BroadcastDeviceIDs, d = broadcastFilterIDs(ctx, p, "DEVICES", "deviceIds")
	diags.Append(d...)
	m.BroadcastTagIDs, d = broadcastFilterIDs(ctx, p, "DEVICE_TAGS", "deviceTagIds")
	diags.Append(d...)

	if m.Type.ValueString() == "IOT_OPTIMIZED" {
		m.BandSteering = types.BoolNull()
		m.FastRoaming = types.BoolNull()
	} else if v := boolAt(p, "bandSteeringEnabled"); !v.IsNull() {
		m.BandSteering = v
	}
	if m.BandSteering.IsUnknown() {
		m.BandSteering = types.BoolNull()
	}

	if _, ok := p["network"].(map[string]any); ok {
		if nid := stringAt(p, "network", "networkId"); !nid.IsNull() {
			m.NetworkID = nid
		}
	} else {
		m.NetworkID = types.StringNull()
	}

	if sec, ok := p["securityConfiguration"].(map[string]any); ok {
		if t := stringAt(sec, "type"); !t.IsNull() {
			m.SecurityType = t
		}
		if fr := boolAt(sec, "fastRoamingEnabled"); !fr.IsNull() {
			m.FastRoaming = fr
		}
		if m.FastRoaming.IsUnknown() {
			m.FastRoaming = types.BoolNull()
		}
		m.PmfMode = stringAt(sec, "pmfMode")
		m.Wpa3FastRoaming = boolAt(sec, "wpa3FastRoamingEnabled")

		if keys, ok := sec["presharedKeys"].([]any); ok && len(keys) > 0 {
			m.PresharedKeys = nil
			m.Passphrase = types.StringNull()
			for _, k := range keys {
				km, ok := k.(map[string]any)
				if !ok {
					continue
				}
				m.PresharedKeys = append(m.PresharedKeys, PresharedKey{
					NetworkID:  types.StringValue(stringAt(km, "network", "networkId").ValueString()),
					Passphrase: types.StringValue(stringAt(km, "passphrase").ValueString()),
				})
			}
		} else {
			// nil, not an empty slice, so Terraform treats it as null
			m.PresharedKeys = nil
			if pass := stringAt(sec, "passphrase"); pass.ValueString() != "" {
				m.Passphrase = pass
			}
		}
	}

	// Strings, because 2.4 cannot be a Terraform number attribute name.
	freqStrs := []string{}
	if list, ok := p["broadcastingFrequenciesGHz"].([]any); ok {
		for _, f := range list {
			if n, ok := f.(float64); ok {
				freqStrs = append(freqStrs, strconv.FormatFloat(n, 'f', -1, 64))
			}
		}
	}
	setVal, d := types.SetValueFrom(ctx, types.StringType, freqStrs)
	diags.Append(d...)
	m.FrequenciesGHz = setVal

	return diags
}
