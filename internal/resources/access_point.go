package resources

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/beezly/terraform-provider-unifi/internal/client"
)

// The Integration API cannot write device settings yet, so this resource uses the legacy API.

var regexpChannel = regexp.MustCompile(`^(auto|[0-9]+(\.5)?)$`)

var _ resource.Resource = &AccessPointResource{}
var _ resource.ResourceWithImportState = &AccessPointResource{}

type AccessPointResource struct {
	client *client.Client
}

func NewAccessPointResource() resource.Resource {
	return &AccessPointResource{}
}

type AccessPointModel struct {
	ID             types.String `tfsdk:"id"`
	MAC            types.String `tfsdk:"mac"`
	OutdoorMode    types.String `tfsdk:"outdoor_mode"`
	Channel2G      types.String `tfsdk:"channel_2g"`
	Channel5G      types.String `tfsdk:"channel_5g"`
	ChannelWidth2G types.Int64  `tfsdk:"channel_width_2g"`
	ChannelWidth5G types.Int64  `tfsdk:"channel_width_5g"`
	TxPowerMode2G  types.String `tfsdk:"tx_power_mode_2g"`
	TxPowerMode5G  types.String `tfsdk:"tx_power_mode_5g"`
}

// radioSettings maps a radio_table entry ("ng" = 2.4 GHz, "na" = 5 GHz) to model fields.
type radioSettings struct {
	radio       string
	channel     *types.String
	width       *types.Int64
	txPowerMode *types.String
}

func (m *AccessPointModel) radios() []radioSettings {
	return []radioSettings{
		{"ng", &m.Channel2G, &m.ChannelWidth2G, &m.TxPowerMode2G},
		{"na", &m.Channel5G, &m.ChannelWidth5G, &m.TxPowerMode5G},
	}
}

func (r *AccessPointResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_access_point"
}

func optionalComputedInt64(description string, validators ...validator.Int64) schema.Int64Attribute {
	return schema.Int64Attribute{
		Optional:            true,
		Computed:            true,
		PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
		Validators:          validators,
		MarkdownDescription: description,
	}
}

func validatedString(description string, validators ...validator.String) schema.StringAttribute {
	a := optionalComputedString(description)
	a.Validators = validators
	return a
}

func (r *AccessPointResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	channel := stringvalidator.RegexMatches(regexpChannel, "must be `auto` or a channel number")
	width := int64validator.OneOf(20, 40, 80, 160, 240, 320)
	txPower := stringvalidator.OneOf("auto", "disabled", "high", "low", "medium")
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages radio settings of an adopted access point through the legacy controller API. " +
			"Only the attributes set here are changed; removing the resource leaves the device untouched.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Legacy device ID.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"mac": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "MAC address of the access point.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"outdoor_mode":     validatedString("Outdoor mode: `default`, `on` or `off`.", stringvalidator.OneOf("default", "off", "on")),
			"channel_2g":       validatedString("2.4 GHz channel: `auto` or a channel number.", channel),
			"channel_5g":       validatedString("5 GHz channel: `auto` or a channel number.", channel),
			"channel_width_2g": optionalComputedInt64("2.4 GHz channel width in MHz.", width),
			"channel_width_5g": optionalComputedInt64("5 GHz channel width in MHz.", width),
			"tx_power_mode_2g": validatedString("2.4 GHz transmit power: `auto`, `low`, `medium`, `high` or `disabled`.", txPower),
			"tx_power_mode_5g": validatedString("5 GHz transmit power: `auto`, `low`, `medium`, `high` or `disabled`.", txPower),
		},
	}
}

func (r *AccessPointResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *AccessPointResource) get(ctx context.Context, mac string) (map[string]any, error) {
	var devices []map[string]any
	if err := r.client.LegacyGet(ctx, "stat/device/"+strings.ToLower(mac), &devices); err != nil {
		return nil, err
	}
	if len(devices) != 1 {
		return nil, fmt.Errorf("expected one device with MAC %s, got %d", mac, len(devices))
	}
	return devices[0], nil
}

// apply reads the device, writes the changed settings and stores the result.
func (r *AccessPointResource) apply(ctx context.Context, plan AccessPointModel) (AccessPointModel, error) {
	current, err := r.get(ctx, plan.MAC.ValueString())
	if err != nil {
		return plan, err
	}
	if payload := accessPointPayload(plan, current); len(payload) > 0 {
		id, _ := current["_id"].(string)
		if err := r.client.LegacyPut(ctx, "rest/device/"+id, payload, nil); err != nil {
			return plan, err
		}
		if current, err = r.get(ctx, plan.MAC.ValueString()); err != nil {
			return plan, err
		}
	}
	accessPointToModel(current, &plan)
	return plan, nil
}

func (r *AccessPointResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan AccessPointModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	state, err := r.apply(ctx, plan)
	if err != nil {
		resp.Diagnostics.AddError("Error configuring access point", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *AccessPointResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state AccessPointModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	device, err := r.get(ctx, state.MAC.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading access point", err.Error())
		return
	}
	accessPointToModel(device, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *AccessPointResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan AccessPointModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	state, err := r.apply(ctx, plan)
	if err != nil {
		resp.Diagnostics.AddError("Error configuring access point", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *AccessPointResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func (r *AccessPointResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("mac"), req, resp)
}

// accessPointPayload returns only the settings that differ from the device, so an unchanged
// plan does not re-provision it.
func accessPointPayload(m AccessPointModel, device map[string]any) map[string]any {
	payload := map[string]any{}
	if known(m.OutdoorMode) && device["outdoor_mode_override"] != m.OutdoorMode.ValueString() {
		payload["outdoor_mode_override"] = m.OutdoorMode.ValueString()
	}

	table, _ := device["radio_table"].([]any)
	changed := false
	newTable := make([]any, 0, len(table))
	for _, entry := range table {
		radio, ok := entry.(map[string]any)
		if !ok {
			newTable = append(newTable, entry)
			continue
		}
		updated := make(map[string]any, len(radio))
		for k, v := range radio {
			updated[k] = v
		}
		for _, s := range m.radios() {
			if radio["radio"] != s.radio {
				continue
			}
			if known(*s.channel) && channelString(radio["channel"]) != s.channel.ValueString() {
				updated["channel"] = channelValue(s.channel.ValueString())
				changed = true
			}
			if known(*s.width) && !int64At(radio, "ht").Equal(*s.width) {
				updated["ht"] = s.width.ValueInt64()
				changed = true
			}
			if known(*s.txPowerMode) && radio["tx_power_mode"] != s.txPowerMode.ValueString() {
				updated["tx_power_mode"] = s.txPowerMode.ValueString()
				changed = true
			}
		}
		newTable = append(newTable, updated)
	}
	if changed {
		payload["radio_table"] = newTable
	}
	return payload
}

func accessPointToModel(device map[string]any, m *AccessPointModel) {
	m.ID = stringAt(device, "_id")
	m.OutdoorMode = stringAt(device, "outdoor_mode_override")
	table, _ := device["radio_table"].([]any)
	for _, s := range m.radios() {
		*s.channel, *s.width, *s.txPowerMode = types.StringNull(), types.Int64Null(), types.StringNull()
		for _, entry := range table {
			radio, ok := entry.(map[string]any)
			if !ok || radio["radio"] != s.radio {
				continue
			}
			if c := channelString(radio["channel"]); c != "" {
				*s.channel = types.StringValue(c)
			}
			*s.width = int64At(radio, "ht")
			*s.txPowerMode = stringAt(radio, "tx_power_mode")
		}
	}
}

// The API returns the channel as a number or as "auto".
func channelString(v any) string {
	switch c := v.(type) {
	case float64:
		return strconv.FormatFloat(c, 'f', -1, 64)
	case string:
		return c
	}
	return ""
}

func channelValue(s string) any {
	if n, err := strconv.ParseFloat(s, 64); err == nil {
		return n
	}
	return s
}
