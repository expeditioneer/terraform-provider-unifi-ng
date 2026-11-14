package resources

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func planIndex(t *testing.T, prior, planned FirewallPolicyModel) types.Int64 {
	t.Helper()
	ctx := context.Background()
	var sr resource.SchemaResponse
	(&FirewallPolicyResource{}).Schema(ctx, resource.SchemaRequest{}, &sr)
	empty := tftypes.NewValue(sr.Schema.Type().TerraformType(ctx), nil)
	state := tfsdk.State{Schema: sr.Schema, Raw: empty}
	plan := tfsdk.Plan{Schema: sr.Schema, Raw: empty}
	if d := state.Set(ctx, &prior); d.HasError() {
		t.Fatal(d)
	}
	if d := plan.Set(ctx, &planned); d.HasError() {
		t.Fatal(d)
	}
	req := planmodifier.Int64Request{State: state, Plan: plan, StateValue: prior.Index, PlanValue: planned.Index}
	resp := planmodifier.Int64Response{PlanValue: planned.Index}
	indexInZonePair{}.PlanModifyInt64(ctx, req, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	return resp.PlanValue
}

func TestIndexKeptWithinZonePair(t *testing.T) {
	prior := readPolicyModel(t, decode(t, policyFixture))
	planned := prior
	planned.Index = types.Int64Unknown()
	planned.Name = types.StringValue("renamed")
	if got := planIndex(t, prior, planned); !got.Equal(prior.Index) {
		t.Errorf("got %v, want prior %v", got, prior.Index)
	}
}

func TestIndexUnknownWhenZonePairChanges(t *testing.T) {
	prior := readPolicyModel(t, decode(t, policyFixture))
	for _, change := range []func(*FirewallPolicyModel){
		func(m *FirewallPolicyModel) {
			m.SourceZoneID = types.StringValue("00000000-0000-0000-0000-000000000099")
		},
		func(m *FirewallPolicyModel) { m.DestinationZoneID = types.StringUnknown() },
	} {
		planned := prior
		planned.Index = types.Int64Unknown()
		change(&planned)
		if got := planIndex(t, prior, planned); !got.IsUnknown() {
			t.Errorf("got %v, want unknown", got)
		}
	}
}
