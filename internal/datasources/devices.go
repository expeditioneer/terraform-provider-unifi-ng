package datasources

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/beezly/terraform-provider-unifi/internal/client"
)

var _ datasource.DataSource = &DevicesDataSource{}

type DevicesDataSource struct {
	client *client.Client
}

func NewDevicesDataSource() datasource.DataSource {
	return &DevicesDataSource{}
}

type DeviceItem struct {
	ID      types.String `tfsdk:"id"`
	Name    types.String `tfsdk:"name"`
	Model   types.String `tfsdk:"model"`
	MAC     types.String `tfsdk:"mac"`
	Adopted types.Bool   `tfsdk:"adopted"`
}

type DevicesModel struct {
	Data []DeviceItem `tfsdk:"data"`
}

type deviceAPIItem struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Model   string `json:"model"`
	MAC     string `json:"mac"`
	Adopted bool   `json:"adopted"`
}

type devicesAPIResponse struct {
	Data []deviceAPIItem `json:"data"`
}

func (d *DevicesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_devices"
}

func (d *DevicesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "List all adopted and pending UniFi devices.",
		Attributes: map[string]schema.Attribute{
			"data": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":      schema.StringAttribute{Computed: true},
						"name":    schema.StringAttribute{Computed: true},
						"model":   schema.StringAttribute{Computed: true},
						"mac":     schema.StringAttribute{Computed: true},
						"adopted": schema.BoolAttribute{Computed: true},
					},
				},
			},
		},
	}
}

func (d *DevicesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *DevicesDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	var result devicesAPIResponse
	path := fmt.Sprintf("/v1/sites/%s/devices", d.client.SiteID())
	if err := d.client.Get(ctx, path, &result); err != nil {
		resp.Diagnostics.AddError("Error reading devices", err.Error())
		return
	}

	var state DevicesModel
	for _, dev := range result.Data {
		state.Data = append(state.Data, DeviceItem{
			ID:      types.StringValue(dev.ID),
			Name:    types.StringValue(dev.Name),
			Model:   types.StringValue(dev.Model),
			MAC:     types.StringValue(dev.MAC),
			Adopted: types.BoolValue(dev.Adopted),
		})
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
