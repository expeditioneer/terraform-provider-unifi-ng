package datasources

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/beezly/terraform-provider-unifi/internal/client"
)

var _ datasource.DataSource = &SitesDataSource{}

type SitesDataSource struct {
	client *client.Client
}

func NewSitesDataSource() datasource.DataSource {
	return &SitesDataSource{}
}

type SiteItem struct {
	ID                types.String `tfsdk:"id"`
	Name              types.String `tfsdk:"name"`
	InternalReference types.String `tfsdk:"internal_reference"`
}

type SitesModel struct {
	Data []SiteItem `tfsdk:"data"`
}

type siteAPIItem struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	InternalReference string `json:"internalReference"`
}

func (d *SitesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_sites"
}

func (d *SitesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "List all UniFi sites.",
		Attributes: map[string]schema.Attribute{
			"data": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":                 schema.StringAttribute{Computed: true},
						"name":               schema.StringAttribute{Computed: true},
						"internal_reference": schema.StringAttribute{Computed: true},
					},
				},
			},
		},
	}
}

func (d *SitesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *SitesDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	sites, err := client.GetAll[siteAPIItem](ctx, d.client, "/v1/sites")
	if err != nil {
		resp.Diagnostics.AddError("Error reading sites", err.Error())
		return
	}

	var state SitesModel
	for _, s := range sites {
		state.Data = append(state.Data, SiteItem{
			ID:                types.StringValue(s.ID),
			Name:              types.StringValue(s.Name),
			InternalReference: types.StringValue(s.InternalReference),
		})
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
