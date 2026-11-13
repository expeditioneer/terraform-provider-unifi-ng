package datasources

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/beezly/terraform-provider-unifi/internal/client"
)

// Legacy settings such as ips reference networks by their legacy ID, which differs from the Integration ID.

var _ datasource.DataSource = &LegacyNetworkDataSource{}

type LegacyNetworkDataSource struct {
	client *client.Client
}

func NewLegacyNetworkDataSource() datasource.DataSource {
	return &LegacyNetworkDataSource{}
}

type LegacyNetworkModel struct {
	ID        types.String `tfsdk:"id"`
	NetworkID types.String `tfsdk:"network_id"`
}

type legacyNetwork struct {
	ID         string `json:"_id"`
	ExternalID string `json:"external_id"`
}

func (d *LegacyNetworkDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_legacy_network"
}

func (d *LegacyNetworkDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up the legacy controller ID of a network, for legacy settings such as `ips`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Legacy network ID.",
			},
			"network_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Integration API network ID.",
			},
		},
	}
}

func (d *LegacyNetworkDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *LegacyNetworkDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state LegacyNetworkModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var networks []legacyNetwork
	if err := d.client.LegacyGet(ctx, "rest/networkconf", &networks); err != nil {
		resp.Diagnostics.AddError("Error reading networks", err.Error())
		return
	}
	id, err := findLegacyNetwork(networks, state.NetworkID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error looking up network", err.Error())
		return
	}
	state.ID = types.StringValue(id)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func findLegacyNetwork(networks []legacyNetwork, networkID string) (string, error) {
	for _, n := range networks {
		if n.ExternalID == networkID {
			return n.ID, nil
		}
	}
	return "", fmt.Errorf("no legacy network for network %s", networkID)
}
