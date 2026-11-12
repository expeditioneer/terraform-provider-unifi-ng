package datasources

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/beezly/terraform-provider-unifi/internal/client"
)

var _ datasource.DataSource = &FirewallZoneDataSource{}

type FirewallZoneDataSource struct {
	client *client.Client
}

func NewFirewallZoneDataSource() datasource.DataSource {
	return &FirewallZoneDataSource{}
}

type FirewallZoneModel struct {
	ID         types.String `tfsdk:"id"`
	Name       types.String `tfsdk:"name"`
	NetworkIDs types.Set    `tfsdk:"network_ids"`
	Origin     types.String `tfsdk:"origin"`
}

type firewallZoneAPIItem struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	NetworkIDs []string `json:"networkIds"`
	Metadata   struct {
		Origin string `json:"origin"`
	} `json:"metadata"`
}

type firewallZonesAPIResponse struct {
	Data []firewallZoneAPIItem `json:"data"`
}

func (d *FirewallZoneDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_firewall_zone"
}

func (d *FirewallZoneDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a firewall zone by name, e.g. the system zones `Internal`, `External`, `Gateway` or `Hotspot`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Zone ID (UUID).",
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Zone name.",
			},
			"network_ids": schema.SetAttribute{
				ElementType:         types.StringType,
				Computed:            true,
				MarkdownDescription: "Networks in the zone.",
			},
			"origin": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "`SYSTEM_DEFINED` or `USER_DEFINED`.",
			},
		},
	}
}

func (d *FirewallZoneDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("Expected *client.Client, got %T", req.ProviderData))
		return
	}
	d.client = c
}

func (d *FirewallZoneDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state FirewallZoneModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var result firewallZonesAPIResponse
	if err := d.client.Get(ctx, fmt.Sprintf("/v1/sites/%s/firewall/zones?limit=200", d.client.SiteID()), &result); err != nil {
		resp.Diagnostics.AddError("Error reading firewall zones", err.Error())
		return
	}

	zone, err := findZone(result.Data, state.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error looking up firewall zone", err.Error())
		return
	}

	state.ID = types.StringValue(zone.ID)
	state.Origin = types.StringValue(zone.Metadata.Origin)
	networkIDs, diags := types.SetValueFrom(ctx, types.StringType, append([]string{}, zone.NetworkIDs...))
	resp.Diagnostics.Append(diags...)
	state.NetworkIDs = networkIDs
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func findZone(zones []firewallZoneAPIItem, name string) (*firewallZoneAPIItem, error) {
	var found *firewallZoneAPIItem
	for i := range zones {
		if zones[i].Name != name {
			continue
		}
		if found != nil {
			return nil, fmt.Errorf("more than one firewall zone named %q", name)
		}
		found = &zones[i]
	}
	if found == nil {
		return nil, fmt.Errorf("no firewall zone named %q", name)
	}
	return found, nil
}
