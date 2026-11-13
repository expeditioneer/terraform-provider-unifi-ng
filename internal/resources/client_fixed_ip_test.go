package resources

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestFindClientIgnoresMACCase(t *testing.T) {
	clients := []map[string]any{{"_id": "1", "mac": "e4:5f:01:be:1f:eb"}}
	if c := findClient(clients, "E4:5F:01:BE:1F:EB"); c == nil || c["_id"] != "1" {
		t.Errorf("got %v", c)
	}
	if c := findClient(clients, "00:00:00:00:00:00"); c != nil {
		t.Errorf("got %v, want nil", c)
	}
}

func TestClientToFixedIPModel(t *testing.T) {
	var m ClientFixedIPModel
	if !clientToFixedIPModel(map[string]any{"_id": "1", "use_fixedip": true, "fixed_ip": "172.17.0.1"}, &m) {
		t.Fatal("reservation not detected")
	}
	if m.ID != types.StringValue("1") || m.FixedIP != types.StringValue("172.17.0.1") {
		t.Errorf("got %+v", m)
	}
	if clientToFixedIPModel(map[string]any{"use_fixedip": false, "fixed_ip": "172.17.0.1"}, &m) {
		t.Error("disabled reservation must be reported as gone")
	}
	if clientToFixedIPModel(nil, &m) {
		t.Error("missing client must be reported as gone")
	}
}
