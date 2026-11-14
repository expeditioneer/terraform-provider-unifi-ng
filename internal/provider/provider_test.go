package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func configure(t *testing.T, attrs map[string]tftypes.Value) provider.ConfigureResponse {
	t.Helper()
	ctx := context.Background()
	p := New("test")()
	var sr provider.SchemaResponse
	p.Schema(ctx, provider.SchemaRequest{}, &sr)
	typ := sr.Schema.Type().TerraformType(ctx).(tftypes.Object)
	vals := map[string]tftypes.Value{}
	for name, at := range typ.AttributeTypes {
		vals[name] = tftypes.NewValue(at, nil)
	}
	for name, v := range attrs {
		vals[name] = v
	}
	req := provider.ConfigureRequest{Config: tfsdk.Config{Schema: sr.Schema, Raw: tftypes.NewValue(typ, vals)}}
	var resp provider.ConfigureResponse
	p.Configure(ctx, req, &resp)
	return resp
}

func TestConfigureFallsBackToEnv(t *testing.T) {
	t.Setenv("UNIFI_API_KEY", "key")
	t.Setenv("UNIFI_API_URL", "https://unifi.invalid")
	t.Setenv("UNIFI_SITE_ID", "site")
	resp := configure(t, nil)
	if resp.Diagnostics.HasError() || resp.ResourceData == nil {
		t.Fatalf("diagnostics: %v", resp.Diagnostics)
	}
}

func TestConfigureReportsMissingAttribute(t *testing.T) {
	t.Setenv("UNIFI_API_KEY", "")
	t.Setenv("UNIFI_API_URL", "")
	t.Setenv("UNIFI_SITE_ID", "")
	resp := configure(t, map[string]tftypes.Value{
		"api_url": tftypes.NewValue(tftypes.String, "https://unifi.invalid"),
		"site_id": tftypes.NewValue(tftypes.String, "site"),
	})
	if got := resp.Diagnostics.ErrorsCount(); got != 1 {
		t.Fatalf("got %d errors, want 1 for api_key: %v", got, resp.Diagnostics)
	}
}

func TestConfigureRejectsUnknown(t *testing.T) {
	t.Setenv("UNIFI_API_KEY", "key")
	t.Setenv("UNIFI_API_URL", "https://unifi.invalid")
	t.Setenv("UNIFI_SITE_ID", "site")
	resp := configure(t, map[string]tftypes.Value{"site_id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue)})
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error for an unknown site_id")
	}
}
