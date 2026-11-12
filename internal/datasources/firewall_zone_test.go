package datasources

import "testing"

func TestFindZone(t *testing.T) {
	zones := []firewallZoneAPIItem{{ID: "1", Name: "Internal"}, {ID: "2", Name: "Hotspot"}}

	if z, err := findZone(zones, "Hotspot"); err != nil || z.ID != "2" {
		t.Errorf("findZone(Hotspot) = %v, %v", z, err)
	}
	if _, err := findZone(zones, "Missing"); err == nil {
		t.Error("expected an error for a missing zone")
	}
	if _, err := findZone(append(zones, firewallZoneAPIItem{ID: "3", Name: "Hotspot"}), "Hotspot"); err == nil {
		t.Error("expected an error for a duplicate name")
	}
}
