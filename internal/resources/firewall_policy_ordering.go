package resources

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/beezly/terraform-provider-unifi/internal/client"
)

var _ resource.Resource = &FirewallPolicyOrderingResource{}
var _ resource.ResourceWithImportState = &FirewallPolicyOrderingResource{}
var _ resource.ResourceWithValidateConfig = &FirewallPolicyOrderingResource{}

type FirewallPolicyOrderingResource struct {
	client *client.Client
}

func NewFirewallPolicyOrderingResource() resource.Resource {
	return &FirewallPolicyOrderingResource{}
}

type FirewallPolicyOrderingModel struct {
	AfterSystemDefined  types.List   `tfsdk:"after_system_defined"`
	BeforeSystemDefined types.List   `tfsdk:"before_system_defined"`
	DestinationZoneID   types.String `tfsdk:"destination_zone_id"`
	ID                  types.String `tfsdk:"id"`
	SourceZoneID        types.String `tfsdk:"source_zone_id"`
}

type policyOrdering struct {
	AfterSystemDefined  []string `json:"afterSystemDefined"`
	BeforeSystemDefined []string `json:"beforeSystemDefined"`
}

type policyOrderingAPI struct {
	OrderedFirewallPolicyIDs policyOrdering `json:"orderedFirewallPolicyIds"`
}

func (r *FirewallPolicyOrderingResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_firewall_policy_ordering"
}

func (r *FirewallPolicyOrderingResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	idList := func(description string) schema.ListAttribute {
		return schema.ListAttribute{
			ElementType:         types.StringType,
			Optional:            true,
			Computed:            true,
			Default:             listdefault.StaticValue(types.ListValueMust(types.StringType, nil)),
			MarkdownDescription: description,
		}
	}
	zoneID := func(description string) schema.StringAttribute {
		return schema.StringAttribute{
			Required:            true,
			MarkdownDescription: description,
			PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
		}
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the order of all user-defined firewall policies of one source/destination zone pair. " +
			"List every policy of the pair: apply keeps unlisted ones at the end of their side, so they show up as drift until added here or deleted. " +
			"Destroying this resource leaves the order on the controller unchanged.",
		Attributes: map[string]schema.Attribute{
			"after_system_defined":  idList("Policy IDs evaluated after the system-defined policies, in order."),
			"before_system_defined": idList("Policy IDs evaluated before the system-defined policies, in order."),
			"destination_zone_id":   zoneID("Destination firewall zone UUID."),
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "`<source_zone_id>/<destination_zone_id>`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"source_zone_id": zoneID("Source firewall zone UUID."),
		},
	}
}

func (r *FirewallPolicyOrderingResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var m FirewallPolicyOrderingModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	seen := map[string]bool{}
	for _, attr := range []struct {
		name string
		list types.List
	}{{"before_system_defined", m.BeforeSystemDefined}, {"after_system_defined", m.AfterSystemDefined}} {
		if attr.list.IsNull() || attr.list.IsUnknown() {
			continue
		}
		for _, e := range attr.list.Elements() {
			s, ok := e.(types.String)
			if !ok || s.IsUnknown() || s.IsNull() {
				continue
			}
			if seen[s.ValueString()] {
				resp.Diagnostics.AddAttributeError(path.Root(attr.name), "Duplicate policy ID",
					fmt.Sprintf("Policy %s is listed more than once.", s.ValueString()))
			}
			seen[s.ValueString()] = true
		}
	}
}

func (r *FirewallPolicyOrderingResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *FirewallPolicyOrderingResource) path(m FirewallPolicyOrderingModel) string {
	q := url.Values{}
	q.Set("destinationFirewallZoneId", m.DestinationZoneID.ValueString())
	q.Set("sourceFirewallZoneId", m.SourceZoneID.ValueString())
	return fmt.Sprintf("/v1/sites/%s/firewall/policies/ordering?%s", r.client.SiteID(), q.Encode())
}

func (r *FirewallPolicyOrderingResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan FirewallPolicyOrderingModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(r.apply(ctx, plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.ID = types.StringValue(plan.SourceZoneID.ValueString() + "/" + plan.DestinationZoneID.ValueString())
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *FirewallPolicyOrderingResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state FirewallPolicyOrderingModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var result policyOrderingAPI
	if err := r.client.Get(ctx, r.path(state), &result); err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading firewall policy ordering", err.Error())
		return
	}

	var d diag.Diagnostics
	state.BeforeSystemDefined, d = types.ListValueFrom(ctx, types.StringType, nonNil(result.OrderedFirewallPolicyIDs.BeforeSystemDefined))
	resp.Diagnostics.Append(d...)
	state.AfterSystemDefined, d = types.ListValueFrom(ctx, types.StringType, nonNil(result.OrderedFirewallPolicyIDs.AfterSystemDefined))
	resp.Diagnostics.Append(d...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *FirewallPolicyOrderingResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan FirewallPolicyOrderingModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(r.apply(ctx, plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete only forgets the resource: every policy of a pair always has a position.
func (r *FirewallPolicyOrderingResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func (r *FirewallPolicyOrderingResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	src, dst, ok := strings.Cut(req.ID, "/")
	if !ok || src == "" || dst == "" {
		resp.Diagnostics.AddError("Invalid import ID", "Expected <source_zone_id>/<destination_zone_id>.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("source_zone_id"), src)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("destination_zone_id"), dst)...)
}

func (r *FirewallPolicyOrderingResource) apply(ctx context.Context, plan FirewallPolicyOrderingModel) diag.Diagnostics {
	var diags diag.Diagnostics
	var want policyOrdering
	diags.Append(plan.BeforeSystemDefined.ElementsAs(ctx, &want.BeforeSystemDefined, false)...)
	diags.Append(plan.AfterSystemDefined.ElementsAs(ctx, &want.AfterSystemDefined, false)...)
	if diags.HasError() {
		return diags
	}

	var current policyOrderingAPI
	if err := r.client.Get(ctx, r.path(plan), &current); err != nil {
		diags.AddError("Error reading firewall policy ordering", err.Error())
		return diags
	}

	body := policyOrderingAPI{OrderedFirewallPolicyIDs: completeOrdering(want, current.OrderedFirewallPolicyIDs)}
	if err := r.client.Put(ctx, r.path(plan), body, nil); err != nil {
		diags.AddError("Error updating firewall policy ordering", err.Error())
	}
	return diags
}

// The API rejects partial lists, and a policy removed from the config still exists while its ordering is updated.
func completeOrdering(want, current policyOrdering) policyOrdering {
	listed := map[string]bool{}
	for _, id := range slices.Concat(want.BeforeSystemDefined, want.AfterSystemDefined) {
		listed[id] = true
	}
	missing := func(ids []string) []string {
		var out []string
		for _, id := range ids {
			if !listed[id] {
				out = append(out, id)
			}
		}
		return out
	}
	return policyOrdering{
		AfterSystemDefined:  nonNil(slices.Concat(want.AfterSystemDefined, missing(current.AfterSystemDefined))),
		BeforeSystemDefined: nonNil(slices.Concat(want.BeforeSystemDefined, missing(current.BeforeSystemDefined))),
	}
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
