package model

// IPInfo is the result of an IP intelligence lookup.
type IPInfo struct {
	IP           string `json:"ip"`
	IsDatacenter bool   `json:"is_datacenter"`
	IsProxy      bool   `json:"is_proxy"`
	IsVPN        bool   `json:"is_vpn"`
	IsTor        bool   `json:"is_tor"`
	ASN          int    `json:"asn"`
	ASNOrg       string `json:"asn_org"`
	ISP          string `json:"isp"`
	Country      string `json:"country"`
	CountryCode  string `json:"country_code"`
	City         string `json:"city"`
	Source       string `json:"source"`
	Cached       bool   `json:"cached"`
}

// ProviderStatus represents the status of an external API provider.
type ProviderStatus struct {
	Name        string `json:"name"`
	Available   bool   `json:"available"`
	RateLimit   int    `json:"rate_limit_per_min"`
	UsedLastMin int    `json:"used_last_min"`
	NeedsKey    bool   `json:"needs_key"`
	HasKey      bool   `json:"has_key"`
}

// StatsResponse is returned by the /stats endpoint.
type StatsResponse struct {
	CacheSize int              `json:"cache_size"`
	CacheTTL  string           `json:"cache_ttl"`
	Providers []ProviderStatus `json:"providers"`
	LocalDB   bool             `json:"local_db_loaded"`
	KnownASNs int              `json:"known_datacenter_asns"`
}

// ErrorResponse is returned on error.
type ErrorResponse struct {
	Error string `json:"error"`
	Code  int    `json:"code"`
}
