package resources

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/beezly/terraform-provider-unifi/internal/client"
)

// The Integration API cannot write site settings yet, so this resource uses the legacy API.

var _ resource.Resource = &SettingDPIResource{}
var _ resource.ResourceWithImportState = &SettingDPIResource{}

type SettingDPIResource struct {
	client *client.Client
}

func NewSettingDPIResource() resource.Resource {
	return &SettingDPIResource{}
}

type SettingDPIModel struct {
	ID                    types.String `tfsdk:"id"`
	Enabled               types.Bool   `tfsdk:"enabled"`
	FingerprintingEnabled types.Bool   `tfsdk:"fingerprinting_enabled"`
}

func (r *SettingDPIResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_setting_dpi"
}

func (r *SettingDPIResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the site's traffic identification (DPI) setting through the legacy controller API. " +
			"Application-based firewall filters require it. Removing the resource leaves the setting untouched.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Legacy setting ID.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"enabled": schema.BoolAttribute{
				Required:            true,
				MarkdownDescription: "Enable traffic identification.",
			},
			"fingerprinting_enabled": optionalComputedBool("Enable device fingerprinting."),
		},
	}
}

func (r *SettingDPIResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *SettingDPIResource) read(ctx context.Context, m *SettingDPIModel) error {
	var settings []map[string]any
	if err := r.client.LegacyGet(ctx, "get/setting/dpi", &settings); err != nil {
		return err
	}
	if len(settings) != 1 {
		return fmt.Errorf("expected one dpi setting, got %d", len(settings))
	}
	m.ID = stringAt(settings[0], "_id")
	m.Enabled = boolAt(settings[0], "enabled")
	m.FingerprintingEnabled = boolAt(settings[0], "fingerprintingEnabled")
	return nil
}

func dpiPayload(m SettingDPIModel) map[string]any {
	p := map[string]any{"enabled": m.Enabled.ValueBool()}
	setBool(p, "fingerprintingEnabled", m.FingerprintingEnabled)
	return p
}

func (r *SettingDPIResource) write(ctx context.Context, plan SettingDPIModel) (SettingDPIModel, error) {
	if err := r.client.LegacyPut(ctx, "set/setting/dpi", dpiPayload(plan), nil); err != nil {
		return plan, err
	}
	return plan, r.read(ctx, &plan)
}

func (r *SettingDPIResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan SettingDPIModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	state, err := r.write(ctx, plan)
	if err != nil {
		resp.Diagnostics.AddError("Error setting DPI", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *SettingDPIResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state SettingDPIModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.read(ctx, &state); err != nil {
		resp.Diagnostics.AddError("Error reading DPI setting", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *SettingDPIResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan SettingDPIModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	state, err := r.write(ctx, plan)
	if err != nil {
		resp.Diagnostics.AddError("Error setting DPI", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *SettingDPIResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func (r *SettingDPIResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
