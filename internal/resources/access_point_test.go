package resources

import (
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

const accessPointFixture = `{
  "_id": "b5de1d02-7932-4d16-9440-91c75c38ce69",
  "mac": "0c:ea:14:bf:fb:b1",
  "outdoor_mode_override": "default",
  "radio_table": [
    {"radio": "ng", "name": "wifi0", "channel": 1, "ht": 20, "tx_power_mode": "auto", "has_dfs": true},
    {"radio": "na", "name": "wifi1", "channel": "auto", "ht": 40, "tx_power_mode": "auto", "has_dfs": true}
  ]
}`

func readAccessPoint(t *testing.T) (map[string]any, AccessPointModel) {
	t.Helper()
	device := decode(t, accessPointFixture)
	var m AccessPointModel
	accessPointToModel(device, &m)
	return device, m
}

func TestAccessPointRead(t *testing.T) {
	_, m := readAccessPoint(t)
	want := AccessPointModel{
		ID:             types.StringValue("b5de1d02-7932-4d16-9440-91c75c38ce69"),
		OutdoorMode:    types.StringValue("default"),
		Channel2G:      types.StringValue("1"),
		Channel5G:      types.StringValue("auto"),
		ChannelWidth2G: types.Int64Value(20),
		ChannelWidth5G: types.Int64Value(40),
		TxPowerMode2G:  types.StringValue("auto"),
		TxPowerMode5G:  types.StringValue("auto"),
	}
	if !reflect.DeepEqual(m, want) {
		t.Errorf("got %+v\nwant %+v", m, want)
	}
}

func TestAccessPointUnchangedPlanWritesNothing(t *testing.T) {
	device, m := readAccessPoint(t)
	if p := accessPointPayload(m, device); len(p) != 0 {
		t.Errorf("got %v, want empty payload", p)
	}
}

func TestAccessPointPayloadChangesOnlyManagedFields(t *testing.T) {
	device, m := readAccessPoint(t)
	m.OutdoorMode = types.StringValue("on")
	m.Channel5G = types.StringValue("36")
	m.ChannelWidth2G = types.Int64Unknown()

	p := accessPointPayload(m, device)
	if p["outdoor_mode_override"] != "on" {
		t.Errorf("outdoor_mode_override = %v", p["outdoor_mode_override"])
	}
	table := p["radio_table"].([]any)
	ng, na := table[0].(map[string]any), table[1].(map[string]any)
	if !reflect.DeepEqual(ng, device["radio_table"].([]any)[0]) {
		t.Errorf("2.4 GHz radio changed: %v", ng)
	}
	if na["channel"] != 36.0 || na["ht"] != 40.0 || na["has_dfs"] != true {
		t.Errorf("5 GHz radio = %v", na)
	}
}
