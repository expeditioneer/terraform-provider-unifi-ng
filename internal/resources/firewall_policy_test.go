package resources

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

const policyFixture = `{
  "id": "00000000-0000-0000-0000-000000000021",
  "enabled": true,
  "name": "Allow IoT MQTT to Home",
  "description": "set in the UI",
  "index": 10000,
  "action": {"type": "ALLOW", "allowReturnTraffic": true},
  "source": {"zoneId": "00000000-0000-0000-0000-000000000022"},
  "destination": {
    "zoneId": "00000000-0000-0000-0000-000000000023",
    "trafficFilter": {
      "type": "PORT",
      "portFilter": {"type": "PORTS", "matchOpposite": false, "items": [{"type": "PORT_NUMBER", "value": 1883}]}
    }
  },
  "ipProtocolScope": {
    "ipVersion": "IPV4",
    "protocolFilter": {"type": "NAMED_PROTOCOL", "protocol": {"name": "TCP"}, "matchOpposite": false}
  },
  "connectionStateFilter": ["ESTABLISHED", "RELATED"],
  "schedule": {"mode": "EVERY_DAY", "timeFilter": {"startTime": "08:00", "stopTime": "20:00"}},
  "loggingEnabled": false,
  "reportFlowEnabled": false,
  "metadata": {"origin": "USER_DEFINED"}
}`

func readPolicyModel(t *testing.T, policy map[string]any) FirewallPolicyModel {
	t.Helper()
	raw, _ := json.Marshal(policy)
	var api firewallPolicyAPI
	if err := json.Unmarshal(raw, &api); err != nil {
		t.Fatal(err)
	}
	var m FirewallPolicyModel
	if err := policyAPIToModel(&api, &m); err != nil {
		t.Fatal(err)
	}
	// Computed without a plan modifier, so unknown during updates.
	m.Index = types.Int64Unknown()
	return m
}

func TestPolicyUpdateRoundTripIsLossless(t *testing.T) {
	current := decode(t, policyFixture)
	payload := updatePayload(current, policyReadOnlyKeys)
	if err := applyPolicyModel(readPolicyModel(t, current), payload); err != nil {
		t.Fatal(err)
	}

	want := decode(t, policyFixture)
	for _, key := range policyReadOnlyKeys {
		delete(want, key)
	}
	if got := normalize(t, payload); !reflect.DeepEqual(got, normalize(t, want)) {
		t.Errorf("payload differs from current object\n got: %v\nwant: %v", got, want)
	}
}

func TestPolicyNullFilterAndProtocolAreRemoved(t *testing.T) {
	current := decode(t, policyFixture)
	model := readPolicyModel(t, current)
	model.DestinationFilter = types.StringNull()
	model.Protocol = types.StringNull()

	payload := updatePayload(current, policyReadOnlyKeys)
	applyPolicyModel(model, payload)
	if _, ok := lookup(payload, "destination", "trafficFilter").(map[string]any); ok {
		t.Error("trafficFilter must be removed")
	}
	if lookup(payload, "ipProtocolScope", "protocolFilter") != nil {
		t.Error("protocolFilter must be removed")
	}
	if payload["description"] != "set in the UI" {
		t.Error("description must be preserved")
	}
}

func TestPolicyBlockDropsAllowReturnTraffic(t *testing.T) {
	current := decode(t, policyFixture)
	model := readPolicyModel(t, current)
	model.ActionType = types.StringValue("BLOCK")

	payload := updatePayload(current, policyReadOnlyKeys)
	applyPolicyModel(model, payload)
	if _, ok := payload["action"].(map[string]any)["allowReturnTraffic"]; ok {
		t.Error("allowReturnTraffic must be removed for BLOCK")
	}
}
