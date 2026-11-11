package resources

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

const networkFixture = `{
  "management": "GATEWAY",
  "id": "00000000-0000-0000-0000-000000000001",
  "name": "Home",
  "enabled": true,
  "vlanId": 1700,
  "default": false,
  "metadata": {"origin": "USER_DEFINED"},
  "zoneId": "00000000-0000-0000-0000-000000000002",
  "isolationEnabled": false,
  "cellularBackupEnabled": true,
  "internetAccessEnabled": true,
  "mdnsForwardingEnabled": true,
  "dhcpGuarding": {"trustedDhcpServerIpAddresses": ["192.0.2.254"]},
  "ipv4Configuration": {
    "autoScaleEnabled": false,
    "hostIpAddress": "192.0.2.254",
    "prefixLength": 24,
    "dhcpConfiguration": {
      "mode": "SERVER",
      "ipAddressRange": {"start": "192.0.2.100", "stop": "192.0.2.150"},
      "leaseTimeSeconds": 86400,
      "domainName": "",
      "pingConflictDetectionEnabled": true,
      "ntpServerIpAddresses": ["192.0.2.1"]
    }
  },
  "ipv6Configuration": {
    "interfaceType": "PREFIX_DELEGATION",
    "clientAddressAssignment": {"slaacEnabled": true},
    "routerAdvertisement": {"priority": "HIGH"},
    "prefixDelegationWanInterfaceId": "00000000-0000-0000-0000-000000000003"
  }
}`

func decode(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func normalize(t *testing.T, v any) any {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func readModel(t *testing.T, network map[string]any) NetworkModel {
	t.Helper()
	var m NetworkModel
	if diags := networkToModel(context.Background(), network, &m); diags.HasError() {
		t.Fatal(diags)
	}
	return m
}

func TestNetworkUpdateRoundTripIsLossless(t *testing.T) {
	current := decode(t, networkFixture)
	model := readModel(t, current)

	payload := networkUpdatePayload(current)
	if diags := applyNetworkModel(context.Background(), model, payload); diags.HasError() {
		t.Fatal(diags)
	}

	want := networkUpdatePayload(current)
	if got := normalize(t, payload); !reflect.DeepEqual(got, normalize(t, want)) {
		t.Fatalf("payload differs from current network\n got: %v\nwant: %v", got, want)
	}
}

func TestNetworkReadDerivesSubnetFromGateway(t *testing.T) {
	model := readModel(t, decode(t, networkFixture))
	if model.IPSubnet.ValueString() != "192.0.2.0/24" || model.GatewayIP.ValueString() != "192.0.2.254" {
		t.Fatalf("got subnet %s gateway %s", model.IPSubnet, model.GatewayIP)
	}
}

func TestNetworkDisableDhcpRemovesConfiguration(t *testing.T) {
	current := decode(t, networkFixture)
	model := readModel(t, current)
	model.DhcpEnabled = types.BoolValue(false)

	payload := networkUpdatePayload(current)
	if diags := applyNetworkModel(context.Background(), model, payload); diags.HasError() {
		t.Fatal(diags)
	}
	if _, ok := payload["ipv4Configuration"].(map[string]any)["dhcpConfiguration"]; ok {
		t.Fatal("dhcpConfiguration still present")
	}
}

func TestNetworkCreateDefaultsGatewayToFirstHost(t *testing.T) {
	model := NetworkModel{
		Name:      types.StringValue("Lab"),
		IPSubnet:  types.StringValue("198.51.100.0/24"),
		GatewayIP: types.StringUnknown(),
	}
	payload := map[string]any{}
	if diags := applyNetworkModel(context.Background(), model, payload); diags.HasError() {
		t.Fatal(diags)
	}
	if got := payload["ipv4Configuration"].(map[string]any)["hostIpAddress"]; got != "198.51.100.1" {
		t.Fatalf("got gateway %v", got)
	}
}
