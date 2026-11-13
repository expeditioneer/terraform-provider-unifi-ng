package datasources

import "testing"

func TestFindLegacyNetwork(t *testing.T) {
	networks := []legacyNetwork{{ID: "legacy-1", ExternalID: "uuid-1"}}
	if id, err := findLegacyNetwork(networks, "uuid-1"); err != nil || id != "legacy-1" {
		t.Errorf("got %q, %v", id, err)
	}
	if _, err := findLegacyNetwork(networks, "uuid-2"); err == nil {
		t.Error("expected an error for an unknown network")
	}
}
