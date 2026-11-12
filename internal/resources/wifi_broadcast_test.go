package resources

import (
	"context"
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

const wifiFixture = `{
  "type": "STANDARD",
  "id": "00000000-0000-0000-0000-000000000011",
  "name": "Home",
  "metadata": {"origin": "USER_DEFINED"},
  "enabled": true,
  "network": {"type": "SPECIFIC", "networkId": "00000000-0000-0000-0000-000000000001"},
  "securityConfiguration": {
    "type": "WPA2_WPA3_PERSONAL",
    "fastRoamingEnabled": false,
    "passphrase": "correct horse battery staple",
    "pmfMode": "OPTIONAL",
    "saeConfiguration": {"anticloggingThresholdSeconds": 5, "syncTimeSeconds": 5},
    "wpa3FastRoamingEnabled": false
  },
  "multicastToUnicastConversionEnabled": true,
  "clientIsolationEnabled": false,
  "hideName": false,
  "uapsdEnabled": false,
  "channel2gLockedTo6": false,
  "dtimPeriod2gLockedTo3": false,
  "dnsAssistanceConfiguration": {"type": "AUTO"},
  "broadcastingFrequenciesGHz": [5, 2.4],
  "bandSteeringEnabled": true,
  "arpProxyEnabled": false,
  "bssTransitionEnabled": false,
  "advertiseDeviceName": false
}`

func readWifiModel(t *testing.T, wifi map[string]any) WifiBroadcastModel {
	t.Helper()
	var m WifiBroadcastModel
	if diags := wifiToModel(context.Background(), wifi, &m); diags.HasError() {
		t.Fatal(diags)
	}
	return m
}

func TestWifiUpdateRoundTripIsLossless(t *testing.T) {
	current := decode(t, wifiFixture)
	model := readWifiModel(t, current)

	payload := updatePayload(current, wifiReadOnlyKeys)
	if diags := applyWifiModel(context.Background(), model, payload); diags.HasError() {
		t.Fatal(diags)
	}

	want := decode(t, wifiFixture)
	delete(want, "id")
	delete(want, "metadata")
	if got := normalize(t, payload); !reflect.DeepEqual(got, normalize(t, want)) {
		t.Errorf("payload differs from current object\n got: %v\nwant: %v", got, want)
	}
}

func TestWifiReadMapsSecurityAndFrequencies(t *testing.T) {
	m := readWifiModel(t, decode(t, wifiFixture))
	var freqs []string
	m.FrequenciesGHz.ElementsAs(context.Background(), &freqs, false)
	if !reflect.DeepEqual(freqs, []string{"2.4", "5"}) && !reflect.DeepEqual(freqs, []string{"5", "2.4"}) {
		t.Errorf("frequencies = %v", freqs)
	}
	if m.PmfMode.ValueString() != "OPTIONAL" || m.Wpa3FastRoaming.ValueBool() {
		t.Errorf("pmf_mode = %v, wpa3_fast_roaming = %v", m.PmfMode, m.Wpa3FastRoaming)
	}
}

func TestWifiBandSteeringFalseOmittedWhenAbsent(t *testing.T) {
	current := decode(t, wifiFixture)
	delete(current, "bandSteeringEnabled")
	model := readWifiModel(t, current)
	model.BandSteering = types.BoolValue(false)

	payload := updatePayload(current, wifiReadOnlyKeys)
	applyWifiModel(context.Background(), model, payload)
	if _, ok := payload["bandSteeringEnabled"]; ok {
		t.Error("bandSteeringEnabled must stay absent")
	}
}

func TestWifiSecurityTypeChangeResetsConfiguration(t *testing.T) {
	current := decode(t, wifiFixture)
	model := readWifiModel(t, current)
	model.SecurityType = types.StringValue("WPA3_PERSONAL")
	model.PmfMode = types.StringUnknown()

	payload := updatePayload(current, wifiReadOnlyKeys)
	applyWifiModel(context.Background(), model, payload)
	sec := payload["securityConfiguration"].(map[string]any)
	if sec["pmfMode"] != "REQUIRED" || sec["passphrase"] != "correct horse battery staple" {
		t.Errorf("securityConfiguration = %v", sec)
	}
}

func TestWifiCreateIncludesRequiredFields(t *testing.T) {
	model := readWifiModel(t, decode(t, wifiFixture))
	model.AdvertiseDeviceName = types.BoolUnknown()
	model.Uapsd = types.BoolUnknown()

	payload := newWifiPayload()
	applyWifiModel(context.Background(), model, payload)
	for _, key := range []string{"advertiseDeviceName", "arpProxyEnabled", "bssTransitionEnabled", "channel2gLockedTo6", "dtimPeriod2gLockedTo3", "uapsdEnabled"} {
		if _, ok := payload[key]; !ok {
			t.Errorf("missing %s", key)
		}
	}
}
