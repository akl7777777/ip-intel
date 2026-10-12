# IP Intel

[中文文档](README_CN.md)

A lightweight, self-hosted IP intelligence API service. Identifies datacenter, proxy, VPN, and Tor IPs using a local ASN database combined with external API fallback.

## Architecture

```
Request → Memory Cache → Local MMDB + ASN List → Persistent Cache (SQLite) → External API Chain → Response
```

**Lookup priority:**
1. In-memory cache (configurable TTL, default 6 hours)
2. Local MMDB for ASN lookup → match against embedded datacenter ASN list (< 1ms, zero external dependency)
3. Persistent cache — SQLite database storing previous API results (optional, survives restarts)
4. External API provider chain (automatic rotation with per-provider rate limiting)

## Quick Start

### Docker (Recommended)

```bash
# 1. Download free ASN database (no registration required)
bash scripts/download-db.sh

# 2. Start the service
docker compose up -d

# 3. Test
curl http://localhost:9090/8.8.8.8
```

### Build from Source

```bash
# Download database
bash scripts/download-db.sh

# Build and run
go build -o ip-intel .
./ip-intel
```

## API Reference

### Lookup IP

```
GET /{ip}
Authorization: Bearer <key>    # Optional, only if AUTH_KEY is set
```

Response `200 OK`:

```json
{
  "ip": "8.8.8.8",
  "is_datacenter": true,
  "is_proxy": false,
  "is_vpn": false,
  "is_tor": false,
  "asn": 15169,
  "asn_org": "Google Cloud",
  "isp": "Google LLC",
  "country": "United States",
  "country_code": "US",
  "city": "Ashburn",
  "source": "local",
  "cached": false
}
```

| Field | Type | Description |
|-------|------|-------------|
| `is_datacenter` | bool | Whether the IP belongs to a datacenter/cloud provider |
| `is_proxy` | bool | Whether the IP is a known proxy |
| `is_vpn` | bool | Whether the IP is a known VPN exit node |
| `is_tor` | bool | Whether the IP is a Tor exit node |
| `asn` | int | Autonomous System Number |
| `asn_org` | string | ASN organization name |
| `isp` | string | Internet Service Provider |
| `country` | string | Country name |
| `country_code` | string | ISO country code |
| `city` | string | City name |
| `source` | string | Data source (`local`, `ip-api`, `ipwhois`, etc.) |
| `cached` | bool | Whether the result was served from cache |
| `strict_status` | string | Strict mode only: `all_clear`, `risk`, or `unavailable` |
| `strict_provider` | string | Strict mode only: source of a risk or unavailable verdict |

### Health Check

```
GET /-/health
```

Returns `{"status": "ok"}`. This endpoint bypasses authentication.

### Service Stats

```
GET /-/stats
```

Returns cache size, provider status, local database status, and known ASN count.
When strict mode is enabled, it also returns `strict_free_providers: true`.

## Configuration

All configuration is done via environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `9090` | Listen port |
| `HOST` | `0.0.0.0` | Listen address |
| `AUTH_KEY` | _(empty)_ | Bearer token for authentication. Empty = no auth |
| `CACHE_TTL_HOURS` | `6` | Cache TTL in hours |
| `MMDB_PATH` | `data/GeoLite2-ASN.mmdb` | Path to MMDB database file |
| `IPINFO_TOKEN` | _(empty)_ | ipinfo.io API token (optional) |
| `IPDATA_API_KEY` | _(empty)_ | ipdata.co API key (optional) |
| `ENABLED_PROVIDERS` | _(empty)_ | Provider priority order, comma-separated |
| `STRICT_FREE_PROVIDERS` | `false` | Strict mode: require every keyless provider to clear the IP |
| `PERSISTENT_CACHE` | `false` | Enable persistent cache for API results |
| `PERSISTENT_CACHE_TYPE` | `sqlite` | Cache backend: `sqlite` or `mysql` |
| `PERSISTENT_CACHE_DSN` | `data/ip-cache.db` | SQLite: file path. MySQL: `user:pass@tcp(host:3306)/dbname` |
| `PERSISTENT_CACHE_TTL_DAYS` | `90` | How long to keep cached results (days) |

## External API Providers

Built-in support for 6 providers with automatic rotation and per-provider rate limiting:

| Provider | Free Tier | Detection | Key Required |
|----------|-----------|-----------|--------------|
| ip-api.com | 45 req/min | Datacenter + Proxy | No |
| ipwhois.io | 10k/month, 45/min | Datacenter + Proxy + VPN + Tor | No |
| freeipapi.com | 60 req/min | Proxy | No |
| ipapi.co | ~30k/month | ASN-based (with local ASN list) | No |
| ipdata.co | 1,500/day | Datacenter + Proxy + VPN + Tor | Yes |
| ipinfo.io | 50k/month | Datacenter + Proxy + VPN + Tor | Yes |

The first 4 free providers are sufficient for normal usage without any API keys.

### Strict Mode

Set `STRICT_FREE_PROVIDERS=true` to query all four keyless providers for public IPs: ip-api, ipwhois, freeipapi, and ipapi.co. The IP passes with `is_proxy=false` and `strict_status="all_clear"` only when every provider returns a verifiable response and neither the local ASN nor any provider reports datacenter, proxy, VPN, or Tor risk. ipapi.co only contributes an ASN-based datacenter check; it does not independently detect proxies.

Any reported risk returns `is_proxy=true`, `strict_status="risk"` immediately. A failed request, rate limit, or incomplete response also returns `is_proxy=true`, but with `strict_status="unavailable"` to distinguish uncertainty from a confirmed signal. `strict_provider` identifies the source. Providers requiring API keys are not queried. Private IPs retain the existing bypass.

An observed free `ipwhois.app` response (2026-10-12) contained `success=true` but no `security` fields. Such responses produce `strict_status="unavailable"` and `strict_provider="ipwhois"`; lookups that reach this provider may therefore remain blocked while those fields are unavailable. HTTP success and complete geolocation data do not substitute for security results.

Strict mode uses separate cache keys from the default mode and does not cache unavailable results. It is off by default; when off, the existing first-success behavior is unchanged. An uncached strict lookup can use up to four requests and consume more free quota.

For Docker Compose, uncomment `STRICT_FREE_PROVIDERS=true` in `docker-compose.yml` and run `docker compose up -d --build`. Set it back to `false` and recreate the container to restore the default mode.

> **Note:** The ip-api.com free tier is for **non-commercial use only**. `ENABLED_PROVIDERS` changes priority but does not exclude providers, and strict mode queries ip-api. Check provider terms before commercial deployment.

## Local Database

### ASN Database (MMDB)

Maps IP addresses to ASN numbers and organization names.

- **Source:** [sapics/ip-location-db](https://github.com/sapics/ip-location-db) — built daily, free, no registration required
- Compatible with MaxMind GeoLite2-ASN format

Download:

```bash
bash scripts/download-db.sh
```

Recommended: set up a weekly cron job to keep the database updated.

### Embedded Datacenter ASN List

The binary includes ~90 verified datacenter/cloud provider ASNs:

- **Major Cloud:** AWS, Azure, GCP, Alibaba Cloud, Tencent Cloud, Oracle Cloud
- **VPS:** DigitalOcean, Vultr, Linode, Hetzner, OVH, Contabo
- **Hosting:** ColoCrossing, Psychz, QuadraNet, Zenlayer
- **CDN:** Cloudflare, Akamai, Fastly

The service works even without the MMDB file — the embedded ASN list combined with the external API chain provides full coverage. The MMDB accelerates lookups by resolving more IPs locally.

## Integration

Call this service from your application:

```go
func LookupIPIntel(ip string) (*IPIntelResult, error) {
    req, _ := http.NewRequest("GET",
        fmt.Sprintf("http://ip-intel:9090/%s", ip), nil)
    req.Header.Set("Authorization", "Bearer YOUR_KEY")

    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()

    var result IPIntelResult
    json.NewDecoder(resp.Body).Decode(&result)
    return &result, nil
}
```

When using Docker Compose, place both services on the same network to access via service name.

## Acknowledgments

This project is made possible by the following open-source projects and free services:

- [sapics/ip-location-db](https://github.com/sapics/ip-location-db) — Free, daily-built IP-to-ASN MMDB database with no registration required
- [oschwald/maxminddb-golang](https://github.com/oschwald/maxminddb-golang) — High-performance MMDB reader for Go
- [ip-api.com](https://ip-api.com/) — Free IP geolocation and proxy detection API
- [ipwhois.io](https://ipwhois.io/) — Free IP intelligence API with VPN/Tor detection
- [freeipapi.com](https://freeipapi.com/) — Free IP geolocation API
- [ipapi.co](https://ipapi.co/) — Free IP address lookup API
- [ipdata.co](https://ipdata.co/) — IP intelligence API with threat detection
- [ipinfo.io](https://ipinfo.io/) — Comprehensive IP data API

Thank you to all the maintainers and contributors of these projects.

## License

MIT
