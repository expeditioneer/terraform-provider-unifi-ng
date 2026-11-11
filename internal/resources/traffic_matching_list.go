package resources

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/beezly/terraform-provider-unifi/internal/client"
)

var _ resource.Resource = &TrafficMatchingListResource{}
var _ resource.ResourceWithImportState = &TrafficMatchingListResource{}

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
	Type  types.String `tfsdk:"type"`  // PORTS | IP_ADDRESSES
	Items types.List   `tfsdk:"items"` // list of strings — port numbers or CIDRs/IPs
}

// tmlAPIItem represents one entry in the list.
type tmlAPIItem struct {
	Type  string `json:"type"`            // PORT_NUMBER | PORT_RANGE | IP_ADDRESS | IP_SUBNET
	Value any    `json:"value,omitempty"` // int for ports, string for IPs
	From  *int   `json:"from,omitempty"`  // port range start
	To    *int   `json:"to,omitempty"`    // port range end
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
				MarkdownDescription: "List type: `PORTS` or `IP_ADDRESSES`.",
			},
			"items": schema.ListAttribute{
				ElementType:         types.StringType,
				Required:            true,
				MarkdownDescription: "List items. For `PORTS`: port numbers or ranges like `8080`, `8000-8999`. For `IP_ADDRESSES`: IPs or CIDRs like `192.168.1.1`, `10.0.0.0/8`.",
			},
		},
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
	if err := r.client.Delete(ctx, fmt.Sprintf("/v1/sites/%s/traffic-matching-lists/%s", r.client.SiteID(), state.ID.ValueString())); err != nil {
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
	for _, s := range strs {
		if m.Type.ValueString() == "PORTS" {
			// Check for range "8000-8999"
			var from, to int
			if n, _ := fmt.Sscanf(s, "%d-%d", &from, &to); n == 2 {
				f, t := from, to
				p.Items = append(p.Items, tmlAPIItem{Type: "PORT_RANGE", From: &f, To: &t})
			} else {
				var port int
				fmt.Sscanf(s, "%d", &port)
				p.Items = append(p.Items, tmlAPIItem{Type: "PORT_NUMBER", Value: port})
			}
		} else {
			// IP_ADDRESSES / IPV6_ADDRESSES — detect subnet vs single IP
			if contains(s, "/") {
				p.Items = append(p.Items, tmlAPIItem{Type: "IP_SUBNET", Value: s})
			} else {
				p.Items = append(p.Items, tmlAPIItem{Type: "IP_ADDRESS", Value: s})
			}
		}
	}
	return p, nil
}

func tmlAPIToModel(ctx context.Context, p *tmlAPI, m *TrafficMatchingListModel) diag.Diagnostics {
	m.ID = types.StringValue(p.ID)
	m.Name = types.StringValue(p.Name)
	m.Type = types.StringValue(p.Type)

	var strs []string
	for _, item := range p.Items {
		switch item.Type {
		case "PORT_NUMBER":
			// Value comes back as float64 from JSON
			switch v := item.Value.(type) {
			case float64:
				strs = append(strs, fmt.Sprintf("%d", int(v)))
			default:
				strs = append(strs, fmt.Sprintf("%v", v))
			}
		case "PORT_RANGE":
			if item.From != nil && item.To != nil {
				strs = append(strs, fmt.Sprintf("%d-%d", *item.From, *item.To))
			}
		case "IP_ADDRESS", "IP_SUBNET", "SUBNET":
			strs = append(strs, fmt.Sprintf("%v", item.Value))
		}
	}
	listVal, d := types.ListValueFrom(ctx, types.StringType, strs)
	m.Items = listVal
	return d
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsAt(s, sub))
}

func containsAt(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
