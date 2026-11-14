# terraform-provider-unifi

A Terraform provider for UniFi Network, built on the **official UniFi Integration API** (OpenAPI-based), falling back to the legacy controller API only where the Integration API has no endpoint.

## Why a new provider?

Existing providers ([paultyng/unifi](https://github.com/paultyng/terraform-provider-unifi), [ubiquiti-community/unifi](https://github.com/ubiquiti-community/terraform-provider-unifi)) use the legacy `/api/s/{site}/rest/` API. This API has become increasingly unreliable on newer UniFi OS firmware (5.x+), with type mismatches causing import failures.

This provider uses the new Integration API (`/v1/sites/{siteId}/...`), which:
- Supports **API key authentication** (no username/password)
- Is **officially documented** via OpenAPI specs
- Is **more stable** across firmware versions

OpenAPI specs sourced from: **<https://beez.ly/unifi-apis/>**

## Resources

| Resource | API Endpoint |
|----------|-------------|
| `unifi_access_point` | legacy `stat/device`, `rest/device` |
| `unifi_client_fixed_ip` | legacy `rest/user` |
| `unifi_firewall_policy` | `/v1/sites/{siteId}/firewall/policies` |
| `unifi_firewall_policy_ordering` | `/v1/sites/{siteId}/firewall/policies/ordering` |
| `unifi_firewall_zone` | `/v1/sites/{siteId}/firewall/zones` |
| `unifi_network` | `/v1/sites/{siteId}/networks` |
| `unifi_setting` | legacy `get/setting`, `set/setting` |
| `unifi_setting_dpi` | legacy `get/setting/dpi`, `set/setting/dpi` |
| `unifi_traffic_matching_list` | `/v1/sites/{siteId}/traffic-matching-lists` |
| `unifi_wifi_broadcast` | `/v1/sites/{siteId}/wifi/broadcasts` |

Settings the Integration API does not expose go through the legacy controller API with the same API key.

## Data Sources

| Data Source | API Endpoint |
|------------|-------------|
| `unifi_devices` | `/v1/sites/{siteId}/devices` |
| `unifi_firewall_zone` | `/v1/sites/{siteId}/firewall/zones` |
| `unifi_legacy_network` | legacy `rest/networkconf` |
| `unifi_sites` | `/v1/sites` |

## Usage

```hcl
provider "unifi" {
  api_key                = var.unifi_api_key # or UNIFI_API_KEY
  api_url                = "https://192.168.0.1" # or UNIFI_API_URL
  site_id                = "88f7af54-98f8-306a-a1c7-c9349722b1f6" # or UNIFI_SITE_ID
  tls_certificate_sha256 = var.unifi_certificate_sha256 # pins the self-signed certificate
}

resource "unifi_network" "iot" {
  name    = "IoT"
  vlan_id = 3
}
```

## Build

```bash
nix develop
make build
go test ./...
```

## Updating for new UniFi API versions

Resources are written by hand. `make fetch` downloads the newest OpenAPI spec from <https://beez.ly/unifi-apis/> that is an official release (per the [release feed](https://community.ui.com/rss/releases/UniFi-Network-Controller/e6712595-81bb-4829-8e42-9e2630fabcfe); beez.ly also lists early access builds) into `specs/` as reference; diff it against the previous version to find API changes.

## Status

Used against UniFi Network 11.0.99 on a UDM SE. There are no acceptance tests; unit tests cover payload building and plan modifiers.
