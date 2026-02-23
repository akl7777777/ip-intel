# IP Intel

轻量级、自部署的 IP 情报查询 API 服务。通过本地 ASN 数据库 + 外部 API 链判断 IP 是否为机房、代理、VPN、Tor。

## 架构

```
请求 → 内存缓存 → 本地 MMDB + ASN 列表 → 外部 API 链 → 返回结果
```

**查询优先级：**
1. 内存缓存（TTL 可配，默认 6 小时）
2. 本地 MMDB 查 ASN → 匹配内嵌机房 ASN 列表（< 1ms，零外部依赖）
3. 外部 API 链（自动轮转，限速保护）

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

> **注意：** ip-api.com 免费版**仅限非商业用途**。商用项目请购买 [ip-api Pro](https://members.ip-api.com/) 或通过 `ENABLED_PROVIDERS=ipwhois,freeipapi,ipapi-co` 排除。

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
