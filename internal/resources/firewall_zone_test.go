package resources

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestZonePayloadAlwaysSendsNetworkIDs(t *testing.T) {
	ctx := context.Background()
	for name, ids := range map[string]types.Set{
		"empty": types.SetValueMust(types.StringType, nil),
		"one":   types.SetValueMust(types.StringType, []attr.Value{types.StringValue("n1")}),
	} {
		t.Run(name, func(t *testing.T) {
			p, d := zoneModelToAPI(ctx, FirewallZoneModel{Name: types.StringValue("IoT"), NetworkIDs: ids})
			if d.HasError() {
				t.Fatal(d)
			}
			raw, _ := json.Marshal(p)
			var got map[string]any
			json.Unmarshal(raw, &got)
			if list, ok := got["networkIds"].([]any); !ok || len(list) != len(ids.Elements()) {
				t.Errorf("got %s", raw)
			}
		})
	}
}
