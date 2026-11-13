package resources

import (
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestDPIPayload(t *testing.T) {
	got := dpiPayload(SettingDPIModel{Enabled: types.BoolValue(true), FingerprintingEnabled: types.BoolUnknown()})
	if want := map[string]any{"enabled": true}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	got = dpiPayload(SettingDPIModel{Enabled: types.BoolValue(false), FingerprintingEnabled: types.BoolValue(true)})
	if want := map[string]any{"enabled": false, "fingerprintingEnabled": true}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
