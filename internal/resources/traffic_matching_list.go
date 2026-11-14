package resources

import (
	"context"
	"fmt"
	"net/netip"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/beezly/terraform-provider-unifi/internal/client"
)

var _ resource.Resource = &TrafficMatchingListResource{}
var _ resource.ResourceWithImportState = &TrafficMatchingListResource{}
var _ resource.ResourceWithValidateConfig = &TrafficMatchingListResource{}

type TrafficMatchingListResource struct {
	client *client.Client
}

func NewTrafficMatchingListResource() resource.Resource {
	return &TrafficMatchingListResource{}
}

// TrafficMatchingListModel is the Terraform schema model.
type TrafficMatchingListModel struct {
	ID    types.String `tfsdk:"id"`
	Name  types.String `tfsdk:"name"`
	Type  types.String `tfsdk:"type"`
	Items types.List   `tfsdk:"items"`
}

// tmlAPIItem holds ports as numbers and addresses as strings; decoded ports are float64, which fmt prints as integers.
type tmlAPIItem struct {
	Start any    `json:"start,omitempty"`
	Stop  any    `json:"stop,omitempty"`
	Type  string `json:"type"`
	Value any    `json:"value,omitempty"`
}

type tmlAPI struct {
	ID    string       `json:"id,omitempty"`
	Name  string       `json:"name"`
	Type  string       `json:"type"`
	Items []tmlAPIItem `json:"items"`
}

func (r *TrafficMatchingListResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_traffic_matching_list"
}

func (r *TrafficMatchingListResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a UniFi traffic matching list (reusable port or IP sets for firewall policies).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "List name.",
			},
			"type": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "List type: `PORTS`, `IPV4_ADDRESSES` or `IPV6_ADDRESSES`.",
				Validators:          []validator.String{stringvalidator.OneOf("IPV4_ADDRESSES", "IPV6_ADDRESSES", "PORTS")},
			},
			"items": schema.ListAttribute{
				ElementType:         types.StringType,
				Required:            true,
				MarkdownDescription: "List items. `PORTS`: `8080` or `8000-8999`. `IPV4_ADDRESSES`: `192.168.1.1`, `10.0.0.0/8` or `192.168.1.10-192.168.1.20`. `IPV6_ADDRESSES`: addresses or subnets in CIDR notation.",
			},
		},
	}
}

func (r *TrafficMatchingListResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var m TrafficMatchingListModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() || !known(m.Type) || m.Items.IsUnknown() {
		return
	}
	for i, e := range m.Items.Elements() {
		if s, ok := e.(types.String); ok && known(s) {
			if _, err := parseTMLItem(m.Type.ValueString(), s.ValueString()); err != nil {
				resp.Diagnostics.AddAttributeError(path.Root("items").AtListIndex(i), "Invalid item", err.Error())
			}
		}
	}
}

func (r *TrafficMatchingListResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("Expected *client.Client, got %T", req.ProviderData))
		return
	}
	r.client = c
}

func (r *TrafficMatchingListResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan TrafficMatchingListModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	payload, d := tmlModelToAPI(ctx, plan)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	var result tmlAPI
	if err := r.client.Post(ctx, fmt.Sprintf("/v1/sites/%s/traffic-matching-lists", r.client.SiteID()), payload, &result); err != nil {
		resp.Diagnostics.AddError("Error creating traffic matching list", err.Error())
		return
	}
	resp.Diagnostics.Append(tmlAPIToModel(ctx, &result, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *TrafficMatchingListResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state TrafficMatchingListModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var result tmlAPI
	if err := r.client.Get(ctx, fmt.Sprintf("/v1/sites/%s/traffic-matching-lists/%s", r.client.SiteID(), state.ID.ValueString()), &result); err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading traffic matching list", err.Error())
		return
	}
	resp.Diagnostics.Append(tmlAPIToModel(ctx, &result, &state)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *TrafficMatchingListResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan TrafficMatchingListModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	payload, d := tmlModelToAPI(ctx, plan)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	var result tmlAPI
	if err := r.client.Put(ctx, fmt.Sprintf("/v1/sites/%s/traffic-matching-lists/%s", r.client.SiteID(), plan.ID.ValueString()), payload, &result); err != nil {
		resp.Diagnostics.AddError("Error updating traffic matching list", err.Error())
		return
	}
	resp.Diagnostics.Append(tmlAPIToModel(ctx, &result, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *TrafficMatchingListResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state TrafficMatchingListModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.Delete(ctx, fmt.Sprintf("/v1/sites/%s/traffic-matching-lists/%s", r.client.SiteID(), state.ID.ValueString())); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting traffic matching list", err.Error())
	}
}

func (r *TrafficMatchingListResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func tmlModelToAPI(ctx context.Context, m TrafficMatchingListModel) (*tmlAPI, diag.Diagnostics) {
	var strs []string
	d := m.Items.ElementsAs(ctx, &strs, false)
	if d.HasError() {
		return nil, d
	}
	p := &tmlAPI{Name: m.Name.ValueString(), Type: m.Type.ValueString()}
	for i, s := range strs {
		item, err := parseTMLItem(p.Type, s)
		if err != nil {
			d.AddAttributeError(path.Root("items").AtListIndex(i), "Invalid item", err.Error())
			continue
		}
		p.Items = append(p.Items, item)
	}
	return p, d
}

func parseTMLItem(listType, s string) (tmlAPIItem, error) {
	start, stop, isRange := strings.Cut(s, "-")
	switch listType {
	case "PORTS":
		if !isRange {
			port, err := parsePort(s)
			return tmlAPIItem{Type: "PORT_NUMBER", Value: port}, err
		}
		from, err := parsePort(start)
		if err != nil {
			return tmlAPIItem{}, err
		}
		to, err := parsePort(stop)
		return tmlAPIItem{Type: "PORT_NUMBER_RANGE", Start: from, Stop: to}, err
	case "IPV4_ADDRESSES", "IPV6_ADDRESSES":
		v4 := listType == "IPV4_ADDRESSES"
		if isRange {
			if !v4 {
				return tmlAPIItem{}, fmt.Errorf("%q: address ranges are only supported in IPV4_ADDRESSES", s)
			}
			for _, a := range []string{start, stop} {
				if addr, err := netip.ParseAddr(a); err != nil || !addr.Is4() {
					return tmlAPIItem{}, fmt.Errorf("%q: %q is not an IPv4 address", s, a)
				}
			}
			return tmlAPIItem{Type: "IP_ADDRESS_RANGE", Start: start, Stop: stop}, nil
		}
		if strings.Contains(s, "/") {
			prefix, err := netip.ParsePrefix(s)
			if err != nil || prefix.Addr().Is4() != v4 {
				return tmlAPIItem{}, fmt.Errorf("%q is not an %s subnet in CIDR notation", s, ipFamily(v4))
			}
			return tmlAPIItem{Type: "SUBNET", Value: s}, nil
		}
		addr, err := netip.ParseAddr(s)
		if err != nil || addr.Is4() != v4 {
			return tmlAPIItem{}, fmt.Errorf("%q is not an %s address", s, ipFamily(v4))
		}
		return tmlAPIItem{Type: "IP_ADDRESS", Value: s}, nil
	}
	return tmlAPIItem{}, fmt.Errorf("unsupported list type %q", listType)
}

func parsePort(s string) (int, error) {
	port, err := strconv.Atoi(s)
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("%q is not a port between 1 and 65535", s)
	}
	return port, nil
}

func ipFamily(v4 bool) string {
	if v4 {
		return "IPv4"
	}
	return "IPv6"
}

func tmlAPIToModel(ctx context.Context, p *tmlAPI, m *TrafficMatchingListModel) diag.Diagnostics {
	var d diag.Diagnostics
	m.ID = types.StringValue(p.ID)
	m.Name = types.StringValue(p.Name)
	m.Type = types.StringValue(p.Type)
	strs := []string{}
	for _, item := range p.Items {
		switch item.Type {
		case "IP_ADDRESS", "PORT_NUMBER", "SUBNET":
			strs = append(strs, fmt.Sprint(item.Value))
		case "IP_ADDRESS_RANGE", "PORT_NUMBER_RANGE":
			strs = append(strs, fmt.Sprintf("%v-%v", item.Start, item.Stop))
		default:
			d.AddError("Unsupported traffic matching list item", fmt.Sprintf("Item type %q is not supported by this provider.", item.Type))
		}
	}
	items, dd := types.ListValueFrom(ctx, types.StringType, strs)
	m.Items = items
	return append(d, dd...)
}
