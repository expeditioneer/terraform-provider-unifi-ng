package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/beezly/terraform-provider-unifi/internal/client"
	"github.com/beezly/terraform-provider-unifi/internal/datasources"
	"github.com/beezly/terraform-provider-unifi/internal/resources"
)

// Ensure UnifiProvider satisfies various provider interfaces.
var _ provider.Provider = &UnifiProvider{}

type UnifiProvider struct {
	version string
}

type UnifiProviderModel struct {
	ApiKey        types.String `tfsdk:"api_key"`
	ApiUrl        types.String `tfsdk:"api_url"`
	SiteId        types.String `tfsdk:"site_id"`
	AllowInsecure types.Bool   `tfsdk:"allow_insecure"`
	CertSHA256    types.String `tfsdk:"tls_certificate_sha256"`
}

// UnifiClient holds shared config passed to resources and data sources.
type UnifiClient struct {
	ApiKey        string
	ApiUrl        string
	SiteId        string
	AllowInsecure bool
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &UnifiProvider{version: version}
	}
}

func (p *UnifiProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "unifi"
	resp.Version = p.version
}

func (p *UnifiProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Interact with UniFi Network via the official Integration API (OpenAPI-based).",
		Attributes: map[string]schema.Attribute{
			"api_key": schema.StringAttribute{
				Required:            true,
				Sensitive:           true,
				MarkdownDescription: "UniFi API key. Generate in: Settings → Integrations → API Keys. Can be set via `UNIFI_API_KEY` env var.",
			},
			"api_url": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "URL of the UniFi controller (e.g. `https://192.168.0.1`). Can be set via `UNIFI_API_URL` env var.",
			},
			"site_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "UniFi site UUID. Find in: Settings → System → Site ID. Can be set via `UNIFI_SITE_ID` env var.",
			},
			"allow_insecure": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Skip TLS certificate verification (for self-signed certs). Defaults to false. Prefer `tls_certificate_sha256`.",
			},
			"tls_certificate_sha256": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Pin the controller certificate by its SHA-256 fingerprint (hex, colons optional), e.g. from `openssl x509 -noout -fingerprint -sha256`. Replaces chain and hostname verification, so it works with the self-signed certificate. Conflicts with `allow_insecure`.",
			},
		},
	}
}

func (p *UnifiProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data UnifiProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	c, err := client.New(
		data.ApiKey.ValueString(),
		data.ApiUrl.ValueString(),
		data.SiteId.ValueString(),
		data.AllowInsecure.ValueBool(),
		data.CertSHA256.ValueString(),
	)
	if err != nil {
		resp.Diagnostics.AddError("Invalid provider configuration", err.Error())
		return
	}

	resp.DataSourceData = c
	resp.ResourceData = c
}

func (p *UnifiProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		resources.NewNetworkResource,
		resources.NewFirewallPolicyResource,
		resources.NewFirewallZoneResource,
		resources.NewTrafficMatchingListResource,
		resources.NewWifiBroadcastResource,
	}
}

func (p *UnifiProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		datasources.NewDevicesDataSource,
		datasources.NewFirewallZoneDataSource,
		datasources.NewSitesDataSource,
	}
}
