package resources

import (
	"reflect"
	"testing"
)

func TestChangedValues(t *testing.T) {
	current := map[string]any{"auto_upgrade": true, "auto_upgrade_hour": 3.0, "server_names": []any{"a", "b"}}
	if got := changedValues(current, map[string]any{"auto_upgrade": true, "auto_upgrade_hour": 3.0}); len(got) != 0 {
		t.Errorf("unchanged values reported: %v", got)
	}
	want := map[string]any{"server_names": []any{"b", "a"}}
	if got := changedValues(current, want); !reflect.DeepEqual(got, want) {
		t.Errorf("list order change not detected: %v", got)
	}
}

func TestSettingValuesKeepsPriorFormatting(t *testing.T) {
	current := map[string]any{"timezone": "Europe/Berlin", "other": 1.0}
	prior := `{"timezone": "Europe/Berlin"}`
	if got, err := settingValues(current, prior); err != nil || got.ValueString() != prior {
		t.Errorf("got %v, %v", got, err)
	}
	current["timezone"] = "UTC"
	if got, _ := settingValues(current, prior); got.ValueString() != `{"timezone":"UTC"}` {
		t.Errorf("got %v", got)
	}
}
