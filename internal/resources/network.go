package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"net"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/beezly/terraform-provider-unifi/internal/client"
)

// Ensure interface compliance.
var _ resource.Resource = &NetworkResource{}
var _ resource.ResourceWithImportState = &NetworkResource{}

// NetworkResource implements the unifi_network resource.
type NetworkResource struct {
	client *client.Client
}

func NewNetworkResource() resource.Resource {
	return &NetworkResource{}
}

// NetworkModel maps to the UniFi network API response.
type NetworkModel struct {
	ID                              types.String `tfsdk:"id"`
	Name                            types.String `tfsdk:"name"`
	Enabled                         types.Bool   `tfsdk:"enabled"`
	VlanID                          types.Int64  `tfsdk:"vlan_id"`
	IPSubnet                        types.String `tfsdk:"ip_subnet"`
	GatewayIP                       types.String `tfsdk:"gateway_ip"`
	DhcpEnabled                     types.Bool   `tfsdk:"dhcp_enabled"`
	DhcpRangeStart                  types.String `tfsdk:"dhcp_range_start"`
	DhcpRangeStop                   types.String `tfsdk:"dhcp_range_stop"`
	DhcpLeaseTime                   types.Int64  `tfsdk:"dhcp_lease_time"`
	DhcpDomainName                  types.String `tfsdk:"dhcp_domain_name"`
	DhcpPingConflictDetection       types.Bool   `tfsdk:"dhcp_ping_conflict_detection"`
	DhcpGuardingTrustedServers      types.Set    `tfsdk:"dhcp_guarding_trusted_servers"`
	InternetAccessEnabled           types.Bool   `tfsdk:"internet_access_enabled"`
	MdnsForwardingEnabled           types.Bool   `tfsdk:"mdns_forwarding_enabled"`
	CellularBackupEnabled           types.Bool   `tfsdk:"cellular_backup_enabled"`
	IsolationEnabled                types.Bool   `tfsdk:"isolation_enabled"`
	IPv6InterfaceType               types.String `tfsdk:"ipv6_interface_type"`
	IPv6PrefixDelegationWanID       types.String `tfsdk:"ipv6_prefix_delegation_wan_id"`
	IPv6SlaacEnabled                types.Bool   `tfsdk:"ipv6_slaac_enabled"`
	IPv6RouterAdvertisementPriority types.String `tfsdk:"ipv6_router_advertisement_priority"`
	ZoneID                          types.String `tfsdk:"zone_id"`
}

// Fields returned by GET that the create/update request does not accept.
var networkReadOnlyKeys = []string{"default", "id", "metadata"}

func (r *NetworkResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_network"
}

func optionalComputedBool(description string) schema.BoolAttribute {
	return schema.BoolAttribute{
		Optional:            true,
		Computed:            true,
		PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
		MarkdownDescription: description,
	}
}

func optionalComputedString(description string) schema.StringAttribute {
	return schema.StringAttribute{
		Optional:            true,
		Computed:            true,
		PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		MarkdownDescription: description,
	}
}

func (r *NetworkResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a UniFi network (VLAN). Updates only change the attributes managed here; all other settings of the network are preserved.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Network ID (UUID).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Network name.",
			},
			"enabled": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
				MarkdownDescription: "Whether the network is enabled.",
			},
			"vlan_id": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "VLAN ID (1–4094). Omit for the default untagged network.",
			},
			"ip_subnet": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "IPv4 subnet in CIDR notation (e.g. `192.168.3.0/24`).",
			},
			"gateway_ip":       optionalComputedString("IPv4 address of the gateway in this network. Defaults to the first host address of `ip_subnet` on create."),
			"dhcp_enabled":     optionalComputedBool("Enable the DHCP server on this network."),
			"dhcp_range_start": optionalComputedString("First address of the DHCP range."),
			"dhcp_range_stop":  optionalComputedString("Last address of the DHCP range."),
			"dhcp_lease_time": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
				MarkdownDescription: "DHCP lease time in seconds.",
			},
			"dhcp_domain_name":             optionalComputedString("Domain name handed out by DHCP."),
			"dhcp_ping_conflict_detection": optionalComputedBool("Ping an address before leasing it to detect conflicts."),
			"dhcp_guarding_trusted_servers": schema.SetAttribute{
				ElementType:         types.StringType,
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Set{setplanmodifier.UseStateForUnknown()},
				MarkdownDescription: "Trusted DHCP server addresses for DHCP guarding. An empty set disables DHCP guarding.",
			},
			"internet_access_enabled":            optionalComputedBool("Allow internet access for clients in this network."),
			"mdns_forwarding_enabled":            optionalComputedBool("Forward mDNS traffic for this network."),
			"cellular_backup_enabled":            optionalComputedBool("Allow cellular backup when all WAN connections are down."),
			"isolation_enabled":                  optionalComputedBool("Isolate this network from all other networks."),
			"ipv6_interface_type":                optionalComputedString("IPv6 interface type: `PREFIX_DELEGATION` or `STATIC`. Null when IPv6 is not configured."),
			"ipv6_prefix_delegation_wan_id":      optionalComputedString("ID of the WAN interface the IPv6 prefix is delegated from."),
			"ipv6_slaac_enabled":                 optionalComputedBool("Allow clients to configure IPv6 addresses via SLAAC."),
			"ipv6_router_advertisement_priority": optionalComputedString("IPv6 router advertisement priority: `LOW`, `MEDIUM` or `HIGH`."),
			"zone_id": schema.StringAttribute{
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
				MarkdownDescription: "Firewall zone the network belongs to. Managed through `unifi_firewall_zone`.",
			},
		},
	}
}

func (r *NetworkResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("Expected *client.Client, got %T", req.ProviderData))
		return
	}
	r.client = c
}

func (r *NetworkResource) networkPath(id string) string {
	return fmt.Sprintf("/v1/sites/%s/networks/%s", r.client.SiteID(), id)
}

func (r *NetworkResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan NetworkModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload := map[string]any{
		"management":        "GATEWAY",
		"ipv4Configuration": map[string]any{"autoScaleEnabled": false},
	}
	resp.Diagnostics.Append(applyNetworkModel(ctx, plan, payload)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var result map[string]any
	if err := r.client.Post(ctx, fmt.Sprintf("/v1/sites/%s/networks", r.client.SiteID()), payload, &result); err != nil {
		resp.Diagnostics.AddError("Error creating network", err.Error())
		return
	}

	resp.Diagnostics.Append(networkToModel(ctx, result, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *NetworkResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state NetworkModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var result map[string]any
	if err := r.client.Get(ctx, r.networkPath(state.ID.ValueString()), &result); err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading network", err.Error())
		return
	}

	resp.Diagnostics.Append(networkToModel(ctx, result, &state)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *NetworkResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan NetworkModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// PUT replaces the whole network, so start from the current object to keep unmanaged settings.
	var current map[string]any
	if err := r.client.Get(ctx, r.networkPath(plan.ID.ValueString()), &current); err != nil {
		resp.Diagnostics.AddError("Error reading network before update", err.Error())
		return
	}
	payload := networkUpdatePayload(current)
	resp.Diagnostics.Append(applyNetworkModel(ctx, plan, payload)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var result map[string]any
	if err := r.client.Put(ctx, r.networkPath(plan.ID.ValueString()), payload, &result); err != nil {
		resp.Diagnostics.AddError("Error updating network", err.Error())
		return
	}

	resp.Diagnostics.Append(networkToModel(ctx, result, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *NetworkResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state NetworkModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.Delete(ctx, r.networkPath(state.ID.ValueString())); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting network", err.Error())
	}
}

func (r *NetworkResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func networkUpdatePayload(current map[string]any) map[string]any {
	return updatePayload(current, networkReadOnlyKeys)
}

// updatePayload returns a deep copy of a GET response without read-only fields.
func updatePayload(current map[string]any, readOnlyKeys []string) map[string]any {
	raw, _ := json.Marshal(current)
	var payload map[string]any
	_ = json.Unmarshal(raw, &payload)
	for _, key := range readOnlyKeys {
		delete(payload, key)
	}
	return payload
}

func known(v attr.Value) bool {
	return !v.IsNull() && !v.IsUnknown()
}

// child returns the nested object at key, creating it if missing.
func child(m map[string]any, key string) map[string]any {
	if c, ok := m[key].(map[string]any); ok {
		return c
	}
	c := map[string]any{}
	m[key] = c
	return c
}

// applyNetworkModel writes every known attribute of the model into the API payload.
func applyNetworkModel(ctx context.Context, m NetworkModel, p map[string]any) diag.Diagnostics {
	var diags diag.Diagnostics

	p["name"] = m.Name.ValueString()
	if known(m.Enabled) {
		p["enabled"] = m.Enabled.ValueBool()
	}
	if known(m.VlanID) {
		p["vlanId"] = m.VlanID.ValueInt64()
	}
	if known(m.InternetAccessEnabled) {
		p["internetAccessEnabled"] = m.InternetAccessEnabled.ValueBool()
	}
	if known(m.MdnsForwardingEnabled) {
		p["mdnsForwardingEnabled"] = m.MdnsForwardingEnabled.ValueBool()
	}
	if known(m.CellularBackupEnabled) {
		p["cellularBackupEnabled"] = m.CellularBackupEnabled.ValueBool()
	}
	if known(m.IsolationEnabled) {
		p["isolationEnabled"] = m.IsolationEnabled.ValueBool()
	}

	ipv4 := child(p, "ipv4Configuration")
	if known(m.IPSubnet) {
		_, subnet, err := net.ParseCIDR(m.IPSubnet.ValueString())
		if err != nil {
			diags.AddError("Invalid ip_subnet", err.Error())
			return diags
		}
		prefix, _ := subnet.Mask.Size()
		ipv4["prefixLength"] = prefix
		gateway, _ := ipv4["hostIpAddress"].(string)
		if known(m.GatewayIP) {
			gateway = m.GatewayIP.ValueString()
		} else if ip := net.ParseIP(gateway); ip == nil || !subnet.Contains(ip) {
			gateway = firstHost(subnet).String()
		}
		ipv4["hostIpAddress"] = gateway
	} else if known(m.GatewayIP) {
		ipv4["hostIpAddress"] = m.GatewayIP.ValueString()
	}

	if known(m.DhcpEnabled) && !m.DhcpEnabled.ValueBool() {
		delete(ipv4, "dhcpConfiguration")
	} else if known(m.DhcpEnabled) || ipv4["dhcpConfiguration"] != nil {
		dhcp := child(ipv4, "dhcpConfiguration")
		if known(m.DhcpEnabled) {
			dhcp["mode"] = "SERVER"
		}
		if known(m.DhcpRangeStart) {
			child(dhcp, "ipAddressRange")["start"] = m.DhcpRangeStart.ValueString()
		}
		if known(m.DhcpRangeStop) {
			child(dhcp, "ipAddressRange")["stop"] = m.DhcpRangeStop.ValueString()
		}
		if known(m.DhcpLeaseTime) {
			dhcp["leaseTimeSeconds"] = m.DhcpLeaseTime.ValueInt64()
		}
		if known(m.DhcpDomainName) {
			dhcp["domainName"] = m.DhcpDomainName.ValueString()
		}
		if known(m.DhcpPingConflictDetection) {
			dhcp["pingConflictDetectionEnabled"] = m.DhcpPingConflictDetection.ValueBool()
		}
	}

	if known(m.DhcpGuardingTrustedServers) {
		var servers []string
		diags.Append(m.DhcpGuardingTrustedServers.ElementsAs(ctx, &servers, false)...)
		if len(servers) == 0 {
			delete(p, "dhcpGuarding")
		} else {
			p["dhcpGuarding"] = map[string]any{"trustedDhcpServerIpAddresses": servers}
		}
	}

	if known(m.IPv6InterfaceType) {
		ipv6 := child(p, "ipv6Configuration")
		ipv6["interfaceType"] = m.IPv6InterfaceType.ValueString()
		if m.IPv6InterfaceType.ValueString() != "PREFIX_DELEGATION" {
			delete(ipv6, "prefixDelegationWanInterfaceId")
		}
		if known(m.IPv6PrefixDelegationWanID) {
			ipv6["prefixDelegationWanInterfaceId"] = m.IPv6PrefixDelegationWanID.ValueString()
		}
		if known(m.IPv6SlaacEnabled) {
			child(ipv6, "clientAddressAssignment")["slaacEnabled"] = m.IPv6SlaacEnabled.ValueBool()
		}
		if known(m.IPv6RouterAdvertisementPriority) {
			child(ipv6, "routerAdvertisement")["priority"] = m.IPv6RouterAdvertisementPriority.ValueString()
		}
	}

	return diags
}

func firstHost(subnet *net.IPNet) net.IP {
	ip := make(net.IP, len(subnet.IP))
	copy(ip, subnet.IP)
	ip[len(ip)-1]++
	return ip
}

func lookup(m map[string]any, keys ...string) any {
	var v any = m
	for _, key := range keys {
		obj, ok := v.(map[string]any)
		if !ok {
			return nil
		}
		v = obj[key]
	}
	return v
}

func stringAt(m map[string]any, keys ...string) types.String {
	if s, ok := lookup(m, keys...).(string); ok {
		return types.StringValue(s)
	}
	return types.StringNull()
}

func boolAt(m map[string]any, keys ...string) types.Bool {
	if b, ok := lookup(m, keys...).(bool); ok {
		return types.BoolValue(b)
	}
	return types.BoolNull()
}

func int64At(m map[string]any, keys ...string) types.Int64 {
	switch n := lookup(m, keys...).(type) {
	case float64:
		return types.Int64Value(int64(n))
	case int64:
		return types.Int64Value(n)
	case int:
		return types.Int64Value(int64(n))
	}
	return types.Int64Null()
}

// networkToModel maps an API network object into the Terraform model.
func networkToModel(ctx context.Context, p map[string]any, m *NetworkModel) diag.Diagnostics {
	var diags diag.Diagnostics

	m.ID = stringAt(p, "id")
	m.Name = stringAt(p, "name")
	if v := boolAt(p, "enabled"); !v.IsNull() {
		m.Enabled = v
	}
	if v := int64At(p, "vlanId"); !v.IsNull() {
		m.VlanID = v
	}
	m.InternetAccessEnabled = boolAt(p, "internetAccessEnabled")
	m.MdnsForwardingEnabled = boolAt(p, "mdnsForwardingEnabled")
	m.CellularBackupEnabled = boolAt(p, "cellularBackupEnabled")
	m.IsolationEnabled = boolAt(p, "isolationEnabled")
	m.ZoneID = stringAt(p, "zoneId")

	m.GatewayIP = stringAt(p, "ipv4Configuration", "hostIpAddress")
	prefix := int64At(p, "ipv4Configuration", "prefixLength")
	if ip := net.ParseIP(m.GatewayIP.ValueString()); ip != nil && !prefix.IsNull() {
		bits := 32
		if ip.To4() == nil {
			bits = 128
		}
		subnet := net.IPNet{IP: ip.Mask(net.CIDRMask(int(prefix.ValueInt64()), bits)), Mask: net.CIDRMask(int(prefix.ValueInt64()), bits)}
		m.IPSubnet = types.StringValue(subnet.String())
	}

	m.DhcpEnabled = types.BoolValue(stringAt(p, "ipv4Configuration", "dhcpConfiguration", "mode").ValueString() == "SERVER")
	m.DhcpRangeStart = stringAt(p, "ipv4Configuration", "dhcpConfiguration", "ipAddressRange", "start")
	m.DhcpRangeStop = stringAt(p, "ipv4Configuration", "dhcpConfiguration", "ipAddressRange", "stop")
	m.DhcpLeaseTime = int64At(p, "ipv4Configuration", "dhcpConfiguration", "leaseTimeSeconds")
	m.DhcpDomainName = stringAt(p, "ipv4Configuration", "dhcpConfiguration", "domainName")
	m.DhcpPingConflictDetection = boolAt(p, "ipv4Configuration", "dhcpConfiguration", "pingConflictDetectionEnabled")

	servers := []string{}
	if list, ok := lookup(p, "dhcpGuarding", "trustedDhcpServerIpAddresses").([]any); ok {
		for _, s := range list {
			if str, ok := s.(string); ok {
				servers = append(servers, str)
			}
		}
	}
	setVal, d := types.SetValueFrom(ctx, types.StringType, servers)
	diags.Append(d...)
	m.DhcpGuardingTrustedServers = setVal

	m.IPv6InterfaceType = stringAt(p, "ipv6Configuration", "interfaceType")
	m.IPv6PrefixDelegationWanID = stringAt(p, "ipv6Configuration", "prefixDelegationWanInterfaceId")
	m.IPv6SlaacEnabled = boolAt(p, "ipv6Configuration", "clientAddressAssignment", "slaacEnabled")
	m.IPv6RouterAdvertisementPriority = stringAt(p, "ipv6Configuration", "routerAdvertisement", "priority")

	return diags
}
