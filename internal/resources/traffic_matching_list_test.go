package resources

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestParseTMLItem(t *testing.T) {
	for _, tc := range []struct {
		listType, in, want string
	}{
		{"PORTS", "443", `{"type":"PORT_NUMBER","value":443}`},
		{"PORTS", "8000-8999", `{"start":8000,"stop":8999,"type":"PORT_NUMBER_RANGE"}`},
		{"IPV4_ADDRESSES", "192.168.1.5", `{"type":"IP_ADDRESS","value":"192.168.1.5"}`},
		{"IPV4_ADDRESSES", "10.0.0.0/8", `{"type":"SUBNET","value":"10.0.0.0/8"}`},
		{"IPV4_ADDRESSES", "192.168.1.10-192.168.1.20", `{"start":"192.168.1.10","stop":"192.168.1.20","type":"IP_ADDRESS_RANGE"}`},
		{"IPV6_ADDRESSES", "2001:db8::1", `{"type":"IP_ADDRESS","value":"2001:db8::1"}`},
		{"IPV6_ADDRESSES", "2001:db8:1::/64", `{"type":"SUBNET","value":"2001:db8:1::/64"}`},
	} {
		item, err := parseTMLItem(tc.listType, tc.in)
		if err != nil {
			t.Errorf("%s %q: %v", tc.listType, tc.in, err)
			continue
		}
		if raw, _ := json.Marshal(item); string(raw) != tc.want {
			t.Errorf("%s %q: got %s, want %s", tc.listType, tc.in, raw, tc.want)
		}
	}
}

func TestParseTMLItemRejects(t *testing.T) {
	for _, tc := range []struct{ listType, in string }{
		{"PORTS", "0"},
		{"PORTS", "65536"},
		{"PORTS", "http"},
		{"PORTS", "80-x"},
		{"IPV4_ADDRESSES", "2001:db8::1"},
		{"IPV4_ADDRESSES", "10.0.0.0/33"},
		{"IPV4_ADDRESSES", "192.168.1.10-2001:db8::1"},
		{"IPV6_ADDRESSES", "192.168.1.5"},
		{"IPV6_ADDRESSES", "2001:db8::1-2001:db8::9"},
		{"IP_ADDRESSES", "192.168.1.5"},
	} {
		if item, err := parseTMLItem(tc.listType, tc.in); err == nil {
			t.Errorf("%s %q: accepted as %+v", tc.listType, tc.in, item)
		}
	}
}

func TestTMLReadRoundTrip(t *testing.T) {
	raw := `{"id":"1","name":"web","type":"PORTS","items":[{"type":"PORT_NUMBER","value":443},{"type":"PORT_NUMBER_RANGE","start":8000,"stop":8999}]}`
	var api tmlAPI
	if err := json.Unmarshal([]byte(raw), &api); err != nil {
		t.Fatal(err)
	}
	var m TrafficMatchingListModel
	if d := tmlAPIToModel(context.Background(), &api, &m); d.HasError() {
		t.Fatal(d)
	}
	want := types.ListValueMust(types.StringType, []attr.Value{types.StringValue("443"), types.StringValue("8000-8999")})
	if !m.Items.Equal(want) {
		t.Errorf("got %v, want %v", m.Items, want)
	}
}
