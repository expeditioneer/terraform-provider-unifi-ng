package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/beezly/terraform-provider-unifi/internal/client"
)

// The Integration API cannot write site settings yet, so this resource uses the legacy API.

var _ resource.Resource = &SettingResource{}

type SettingResource struct {
	client *client.Client
}

func NewSettingResource() resource.Resource {
	return &SettingResource{}
}

type SettingModel struct {
	ID     types.String `tfsdk:"id"`
	Key    types.String `tfsdk:"key"`
	Values types.String `tfsdk:"values"`
}

func (r *SettingResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_setting"
}

func (r *SettingResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages fields of a site setting (e.g. `ntp`, `country`) through the legacy controller API. " +
			"Only the fields in `values` are compared and written, and only when they differ. " +
			"Removing the resource leaves the setting untouched.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Legacy setting ID.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"key": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Setting key, e.g. `ntp`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"values": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "JSON object with the managed fields, e.g. `jsonencode({ timezone = \"Europe/Berlin\" })`.",
			},
		},
	}
}

func (r *SettingResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *SettingResource) get(ctx context.Context, key string) (map[string]any, error) {
	var settings []map[string]any
	if err := r.client.LegacyGet(ctx, "get/setting/"+key, &settings); err != nil {
		return nil, err
	}
	if len(settings) != 1 {
		return nil, fmt.Errorf("expected one %s setting, got %d", key, len(settings))
	}
	return settings[0], nil
}

func decodeValues(s string) (map[string]any, error) {
	var v map[string]any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return nil, fmt.Errorf("values must be a JSON object: %w", err)
	}
	return v, nil
}

// changedValues returns the desired fields that differ from the current setting.
func changedValues(current, desired map[string]any) map[string]any {
	changed := map[string]any{}
	for k, v := range desired {
		if !reflect.DeepEqual(current[k], v) {
			changed[k] = v
		}
	}
	return changed
}

// settingValues returns the current values of the managed fields, keeping prior's formatting when equal.
func settingValues(current map[string]any, prior string) (types.String, error) {
	managed, err := decodeValues(prior)
	if err != nil {
		return types.StringNull(), err
	}
	if len(changedValues(current, managed)) == 0 {
		return types.StringValue(prior), nil
	}
	subset := map[string]any{}
	for k := range managed {
		if v, ok := current[k]; ok {
			subset[k] = v
		}
	}
	b, err := json.Marshal(subset)
	return types.StringValue(string(b)), err
}

func (r *SettingResource) apply(ctx context.Context, plan SettingModel) (SettingModel, error) {
	key := plan.Key.ValueString()
	desired, err := decodeValues(plan.Values.ValueString())
	if err != nil {
		return plan, err
	}
	current, err := r.get(ctx, key)
	if err != nil {
		return plan, err
	}
	// Some settings rotate secrets on every write (mgmt regenerates x_api_token), so skip no-op writes.
	if changed := changedValues(current, desired); len(changed) > 0 {
		if err := r.client.LegacyPut(ctx, "set/setting/"+key, changed, nil); err != nil {
			return plan, err
		}
		if current, err = r.get(ctx, key); err != nil {
			return plan, err
		}
	}
	plan.ID = stringAt(current, "_id")
	plan.Values, err = settingValues(current, plan.Values.ValueString())
	return plan, err
}

func (r *SettingResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan SettingModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	state, err := r.apply(ctx, plan)
	if err != nil {
		resp.Diagnostics.AddError("Error writing setting", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *SettingResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state SettingModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	current, err := r.get(ctx, state.Key.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading setting", err.Error())
		return
	}
	state.ID = stringAt(current, "_id")
	if state.Values, err = settingValues(current, state.Values.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error reading setting", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *SettingResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan SettingModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	state, err := r.apply(ctx, plan)
	if err != nil {
		resp.Diagnostics.AddError("Error writing setting", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *SettingResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}
