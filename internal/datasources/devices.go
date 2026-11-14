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
	FirmwareVersion types.String `tfsdk:"firmware_version"`
	ID              types.String `tfsdk:"id"`
	IPAddress       types.String `tfsdk:"ip_address"`
	MAC             types.String `tfsdk:"mac"`
	Model           types.String `tfsdk:"model"`
	Name            types.String `tfsdk:"name"`
	State           types.String `tfsdk:"state"`
}

type DevicesModel struct {
	Data []DeviceItem `tfsdk:"data"`
}

type deviceAPIItem struct {
	FirmwareVersion string `json:"firmwareVersion"`
	ID              string `json:"id"`
	IPAddress       string `json:"ipAddress"`
	MAC             string `json:"macAddress"`
	Model           string `json:"model"`
	Name            string `json:"name"`
	State           string `json:"state"`
}

func (d *DevicesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_devices"
}

func (d *DevicesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists the adopted UniFi devices of the site.",
		Attributes: map[string]schema.Attribute{
			"data": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"firmware_version": schema.StringAttribute{Computed: true},
						"id":               schema.StringAttribute{Computed: true},
						"ip_address":       schema.StringAttribute{Computed: true},
						"mac":              schema.StringAttribute{Computed: true},
						"model":            schema.StringAttribute{Computed: true},
						"name":             schema.StringAttribute{Computed: true},
						"state":            schema.StringAttribute{Computed: true, MarkdownDescription: "E.g. `ONLINE`, `OFFLINE`, `UPDATING`."},
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
	devices, err := client.GetAll[deviceAPIItem](ctx, d.client, fmt.Sprintf("/v1/sites/%s/devices", d.client.SiteID()))
	if err != nil {
		resp.Diagnostics.AddError("Error reading devices", err.Error())
		return
	}

	var state DevicesModel
	for _, dev := range devices {
		state.Data = append(state.Data, DeviceItem{
			FirmwareVersion: types.StringValue(dev.FirmwareVersion),
			ID:              types.StringValue(dev.ID),
			IPAddress:       types.StringValue(dev.IPAddress),
			MAC:             types.StringValue(dev.MAC),
			Model:           types.StringValue(dev.Model),
			Name:            types.StringValue(dev.Name),
			State:           types.StringValue(dev.State),
		})
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
