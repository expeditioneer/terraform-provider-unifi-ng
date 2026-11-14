# terraform-provider-unifi

A Terraform provider for UniFi Network, built on the **official UniFi Integration API** (OpenAPI-based) rather than the legacy controller API.

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
| `unifi_network` | `/v1/sites/{siteId}/networks` |
| `unifi_firewall_policy` | `/v1/sites/{siteId}/firewall/policies` |
| `unifi_firewall_policy_ordering` | `/v1/sites/{siteId}/firewall/policies/ordering` |
| `unifi_firewall_zone` | `/v1/sites/{siteId}/firewall/zones` |
| `unifi_dns_policy` | `/v1/sites/{siteId}/dns/policies` |
| `unifi_traffic_matching_list` | `/v1/sites/{siteId}/traffic-matching-lists` |

## Data Sources

| Data Source | API Endpoint |
|------------|-------------|
| `unifi_sites` | `/v1/sites` |
| `unifi_devices` | `/v1/sites/{siteId}/devices` |

## Usage

```hcl
provider "unifi" {
  api_key        = var.unifi_api_key   # or UNIFI_API_KEY env var
  api_url        = "https://192.168.0.1"
  site_id        = "88f7af54-98f8-306a-a1c7-c9349722b1f6"
  allow_insecure = true  # for self-signed certs
}

resource "unifi_network" "iot" {
  name    = "IoT"
  vlan_id = 3
}
```

## Build

```bash
make build
```

## Updating for new UniFi API versions

Resources are written by hand. `make fetch` downloads the latest OpenAPI spec from <https://beez.ly/unifi-apis/> into `specs/` as reference; diff it against the previous version to find API changes.

## Status

🚧 **Work in progress** — schema generation is working, HTTP CRUD implementations in progress.
