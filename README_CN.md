# IP Intel

轻量级、自部署的 IP 情报查询 API 服务。通过本地 ASN 数据库 + 外部 API 链判断 IP 是否为机房、代理、VPN、Tor。

## 架构

```
请求 → 内存缓存 → 本地 MMDB + ASN 列表 → 持久化缓存（SQLite/MySQL）→ 外部 API 链 → 返回结果
```

**查询优先级：**
1. 内存缓存（TTL 可配，默认 6 小时）
2. 本地 MMDB 查 ASN → 匹配内嵌机房 ASN 列表（< 1ms，零外部依赖）
3. 持久化缓存 — SQLite 或 MySQL 数据库存储历史 API 查询结果（可选，重启不丢失）
4. 外部 API 链（自动轮转，限速保护）

## 快速开始

### Docker 部署（推荐）

```bash
# 1. 下载免费 ASN 数据库（无需注册）
bash scripts/download-db.sh

# 2. 启动
docker compose up -d

# 3. 测试
curl http://localhost:9090/8.8.8.8
```

### 本地编译运行

```bash
# 下载数据库
bash scripts/download-db.sh

# 编译运行
go build -o ip-intel .
./ip-intel
```

## API 接口

### 查询 IP

```
GET /{ip}
Authorization: Bearer <密钥>    # 可选，仅设置 AUTH_KEY 时需要
```

响应 `200 OK`：

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

| 字段 | 类型 | 说明 |
|------|------|------|
| `is_datacenter` | bool | 是否机房/云服务商 IP |
| `is_proxy` | bool | 是否代理 IP |
| `is_vpn` | bool | 是否 VPN 出口 |
| `is_tor` | bool | 是否 Tor 出口 |
| `asn` | int | 自治系统编号 |
| `asn_org` | string | ASN 组织名称 |
| `isp` | string | 网络服务提供商 |
| `country` | string | 国家名称 |
| `country_code` | string | ISO 国家代码 |
| `city` | string | 城市名称 |
| `source` | string | 数据来源（`local`、`ip-api`、`ipwhois` 等） |
| `cached` | bool | 是否命中缓存 |
| `strict_status` | string | 仅严格模式返回：`all_clear`、`risk` 或 `unavailable` |
| `strict_provider` | string | 仅严格模式命中风险或无法确认时，返回对应来源 |

### 健康检查

```
GET /-/health
```

返回 `{"status": "ok"}`。该接口不需要鉴权。

### 服务状态

```
GET /-/stats
```

返回缓存大小、Provider 状态、本地数据库状态等信息。
严格模式开启时还返回 `strict_free_providers: true`。

## 配置

通过环境变量配置：

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `PORT` | `9090` | 监听端口 |
| `HOST` | `0.0.0.0` | 监听地址 |
| `AUTH_KEY` | _空_ | Bearer Token 鉴权密钥，留空则不鉴权 |
| `CACHE_TTL_HOURS` | `6` | 缓存有效期（小时） |
| `MMDB_PATH` | `data/GeoLite2-ASN.mmdb` | MMDB 数据库路径 |
| `IPINFO_TOKEN` | _空_ | ipinfo.io API Token（可选） |
| `IPDATA_API_KEY` | _空_ | ipdata.co API Key（可选） |
| `ENABLED_PROVIDERS` | _空_ | Provider 优先顺序，逗号分隔 |
| `STRICT_FREE_PROVIDERS` | `false` | 严格模式：所有免密 Provider 都确认无风险才通过 |
| `PERSISTENT_CACHE` | `false` | 启用持久化缓存（存储 API 查询结果） |
| `PERSISTENT_CACHE_TYPE` | `sqlite` | 缓存后端：`sqlite` 或 `mysql` |
| `PERSISTENT_CACHE_DSN` | `data/ip-cache.db` | SQLite：文件路径；MySQL：`user:pass@tcp(host:3306)/dbname` |
| `PERSISTENT_CACHE_TTL_DAYS` | `90` | 缓存条目保留天数 |

## 外部 API Provider

内置 6 个 Provider，自动轮转和限速保护：

| Provider | 免费额度 | 检测能力 | 需要 Key |
|----------|----------|----------|----------|
| ip-api.com | 45次/分钟 | 机房 + 代理 | 否 |
| ipwhois.io | 1万/月, 45/分钟 | 机房 + 代理 + VPN + Tor | 否 |
| freeipapi.com | 60次/分钟 | 代理 | 否 |
| ipapi.co | ~3万/月 | ASN（配合本地 ASN 列表判机房） | 否 |
| ipdata.co | 1500次/天 | 机房 + 代理 + VPN + Tor | 是 |
| ipinfo.io | 5万/月 | 机房 + 代理 + VPN + Tor | 是 |

不配置 API Key 的情况下，前 4 个免费 Provider 足够日常使用。

### 严格模式

设置 `STRICT_FREE_PROVIDERS=true` 后，公网 IP 会按顺序查询全部 4 个免密 Provider：ip-api、ipwhois、freeipapi、ipapi.co。只有四家都成功返回可验证的结果，且本地 ASN 与上游均未标记机房、代理、VPN 或 Tor，才返回 `is_proxy=false`、`strict_status="all_clear"`。ipapi.co 只提供 ASN 风险检查，不具备独立的代理检测能力。

任一来源报告风险时立即返回 `is_proxy=true`、`strict_status="risk"`；请求失败、超限或缺少必要字段时同样返回 `is_proxy=true`，但 `strict_status="unavailable"` 表明这是无法确认，不代表已证实代理。`strict_provider` 标识触发的来源。带 Key 的 Provider 不参与严格模式。私有地址仍按原有规则直接返回。

当前 `ipwhois.app` 免费接口的实测响应（2026-10-12）虽然包含 `success=true`，却没有 `security` 字段。这类响应会返回 `strict_status="unavailable"`、`strict_provider="ipwhois"`，因此启用严格模式后，查询走到该来源时可能持续无法通过。HTTP 成功或地理信息完整不能替代安全检测结果。

严格模式与默认模式使用不同的缓存键，不复用旧判定；无法确认的结果不缓存。此开关默认关闭，关闭时保持原有“首个成功结果即返回”的行为。开启后一次未缓存查询最多请求四家服务，耗时和免费额度消耗会增加。

Docker Compose 部署时取消 `docker-compose.yml` 中 `STRICT_FREE_PROVIDERS=true` 的注释，再运行 `docker compose up -d --build`。改回 `false` 并重建容器即可恢复默认模式。

> **注意：** ip-api.com 免费版**仅限非商业用途**。`ENABLED_PROVIDERS` 只调整顺序，不会排除 Provider；严格模式会查询 ip-api。商用部署需要先确保所用服务的许可适用。

## 本地数据库

### ASN 数据库（MMDB）

用于将 IP 映射到 ASN 编号和组织名。

- **数据来源：**[sapics/ip-location-db](https://github.com/sapics/ip-location-db) — 每日构建，免费，无需注册
- 兼容 MaxMind GeoLite2-ASN 格式

下载：

```bash
bash scripts/download-db.sh
```

建议通过 cron 每周更新一次。

### 内嵌机房 ASN 列表

程序内嵌了 ~90 个确认的机房/云服务商 ASN：

- 主流云：AWS、Azure、GCP、阿里云、腾讯云、Oracle Cloud
- VPS：DigitalOcean、Vultr、Linode、Hetzner、OVH、Contabo
- 托管：ColoCrossing、Psychz、QuadraNet、Zenlayer
- CDN：Cloudflare、Akamai、Fastly

即使没有 MMDB 文件，内嵌 ASN 列表 + 外部 API 链仍可正常工作。MMDB 只是加速查询，让更多 IP 可以在本地直接判定。

## 集成示例

在你的应用中调用此服务：

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

Docker Compose 中将两个服务放在同一网络即可通过服务名互访。

## 致谢

本项目的实现依赖于以下开源项目和免费服务，在此表示感谢：

- [sapics/ip-location-db](https://github.com/sapics/ip-location-db) — 免费、每日构建的 IP-ASN MMDB 数据库，无需注册
- [oschwald/maxminddb-golang](https://github.com/oschwald/maxminddb-golang) — 高性能 MMDB 读取库
- [ip-api.com](https://ip-api.com/) — 免费 IP 定位及代理检测 API
- [ipwhois.io](https://ipwhois.io/) — 免费 IP 情报 API，支持 VPN/Tor 检测
- [freeipapi.com](https://freeipapi.com/) — 免费 IP 定位 API
- [ipapi.co](https://ipapi.co/) — 免费 IP 查询 API
- [ipdata.co](https://ipdata.co/) — IP 情报 API，含威胁检测
- [ipinfo.io](https://ipinfo.io/) — 综合 IP 数据 API

感谢以上项目的维护者和贡献者。

## License

MIT
