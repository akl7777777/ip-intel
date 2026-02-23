package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"
)

// Provider is an external IP intelligence API.
type Provider struct {
	Name      string
	QueryFn   func(ip string) (*IPInfo, error)
	RateLimit int // max requests per minute, 0 = needs API key
	NeedsKey  bool
	HasKey    bool

	mu        sync.Mutex
	callTimes []int64
}

// Available returns true if the provider can accept a request.
func (p *Provider) Available() bool {
	if p.NeedsKey && !p.HasKey {
		return false
	}
	if p.RateLimit <= 0 {
		return p.HasKey
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now().Unix()
	cutoff := now - 60

	// Clean old entries
	valid := p.callTimes[:0]
	for _, t := range p.callTimes {
		if t > cutoff {
			valid = append(valid, t)
		}
	}
	p.callTimes = valid

	return len(p.callTimes) < p.RateLimit
}

// RecordCall records a call timestamp.
func (p *Provider) RecordCall() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.callTimes = append(p.callTimes, time.Now().Unix())
}

// UsedLastMinute returns how many calls were made in the last minute.
func (p *Provider) UsedLastMinute() int {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now().Unix()
	cutoff := now - 60
	count := 0
	for _, t := range p.callTimes {
		if t > cutoff {
			count++
		}
	}
	return count
}

var httpClient = &http.Client{Timeout: 5 * time.Second}

func fetchJSON(url string, target interface{}) error {
	resp, err := httpClient.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	return json.NewDecoder(resp.Body).Decode(target)
}

// ---- Provider Implementations ----

// ip-api.com — 45 req/min free, no key needed
func queryIPAPI(ip string) (*IPInfo, error) {
	url := fmt.Sprintf("http://ip-api.com/json/%s?fields=status,message,country,countryCode,city,isp,org,as,hosting,proxy", ip)

	var resp struct {
		Status      string `json:"status"`
		Message     string `json:"message"`
		Country     string `json:"country"`
		CountryCode string `json:"countryCode"`
		City        string `json:"city"`
		ISP         string `json:"isp"`
		Org         string `json:"org"`
		AS          string `json:"as"`
		Hosting     bool   `json:"hosting"`
		Proxy       bool   `json:"proxy"`
	}

	if err := fetchJSON(url, &resp); err != nil {
		return nil, err
	}
	if resp.Status != "success" {
		return nil, fmt.Errorf("ip-api error: %s", resp.Message)
	}

	asn := parseASN(resp.AS)

	return &IPInfo{
		IP:           ip,
		IsDatacenter: resp.Hosting,
		IsProxy:      resp.Proxy,
		ASN:          asn,
		ASNOrg:       resp.Org,
		ISP:          resp.ISP,
		Country:      resp.Country,
		CountryCode:  resp.CountryCode,
		City:         resp.City,
		Source:       "ip-api",
	}, nil
}

// ipwhois.io — 10k/month, 45/min, security=1 needed for proxy/vpn/tor
func queryIPWhois(ip string) (*IPInfo, error) {
	url := fmt.Sprintf("https://ipwhois.app/json/%s?security=1", ip)

	var resp struct {
		Success     bool   `json:"success"`
		Country     string `json:"country"`
		CountryCode string `json:"country_code"`
		City        string `json:"city"`
		ISP         string `json:"isp"`
		Org         string `json:"org"`
		ASN         string `json:"asn"`
		Security    struct {
			Anonymous bool `json:"anonymous"`
			Proxy     bool `json:"proxy"`
			VPN       bool `json:"vpn"`
			Tor       bool `json:"tor"`
			Hosting   bool `json:"hosting"`
		} `json:"security"`
	}

	if err := fetchJSON(url, &resp); err != nil {
		return nil, err
	}

	asn := parseASN(resp.ASN)

	return &IPInfo{
		IP:           ip,
		IsDatacenter: resp.Security.Hosting,
		IsProxy:      resp.Security.Proxy || resp.Security.Anonymous,
		IsVPN:        resp.Security.VPN,
		IsTor:        resp.Security.Tor,
		ASN:          asn,
		ASNOrg:       resp.Org,
		ISP:          resp.ISP,
		Country:      resp.Country,
		CountryCode:  resp.CountryCode,
		City:         resp.City,
		Source:       "ipwhois",
	}, nil
}

// freeipapi.com — 60 req/min free
func queryFreeIPAPI(ip string) (*IPInfo, error) {
	url := fmt.Sprintf("https://freeipapi.com/api/json/%s", ip)

	var resp struct {
		CountryName string  `json:"countryName"`
		CountryCode string  `json:"countryCode"`
		CityName    string  `json:"cityName"`
		IsProxy     bool    `json:"isProxy"`
		Latitude    float64 `json:"latitude"`
		Longitude   float64 `json:"longitude"`
	}

	if err := fetchJSON(url, &resp); err != nil {
		return nil, err
	}

	return &IPInfo{
		IP:          ip,
		IsProxy:     resp.IsProxy,
		Country:     resp.CountryName,
		CountryCode: resp.CountryCode,
		City:        resp.CityName,
		Source:      "freeipapi",
	}, nil
}

// ipapi.co — ~30k/month free, ~1k/day
func queryIPAPICo(ip string) (*IPInfo, error) {
	url := fmt.Sprintf("https://ipapi.co/%s/json/", ip)

	var resp struct {
		Country     string `json:"country_name"`
		CountryCode string `json:"country_code"`
		City        string `json:"city"`
		Org         string `json:"org"`
		ASN         string `json:"asn"`
		Version     string `json:"version"`
	}

	if err := fetchJSON(url, &resp); err != nil {
		return nil, err
	}

	asn := parseASN(resp.ASN)

	info := &IPInfo{
		IP:          ip,
		ASN:         asn,
		ASNOrg:      resp.Org,
		ISP:         resp.Org,
		Country:     resp.Country,
		CountryCode: resp.CountryCode,
		City:        resp.City,
		Source:      "ipapi-co",
	}

	// No direct hosting/proxy field in free tier, check ASN list
	if _, ok := IsKnownDatacenterASN(asn); ok {
		info.IsDatacenter = true
	}

	return info, nil
}

// ipdata.co — needs API key, 1500/day free
func queryIPData(ip string, apiKey string) func(string) (*IPInfo, error) {
	return func(ipAddr string) (*IPInfo, error) {
		url := fmt.Sprintf("https://api.ipdata.co/%s?api-key=%s", ipAddr, apiKey)

		var resp struct {
			Country     string `json:"country_name"`
			CountryCode string `json:"country_code"`
			City        string `json:"city"`
			ASN         struct {
				ASN    string `json:"asn"`
				Name   string `json:"name"`
				Domain string `json:"domain"`
				Type   string `json:"type"` // "hosting", "isp", "business", "education"
			} `json:"asn"`
			Threat struct {
				IsDatacenter       bool `json:"is_datacenter"`
				IsProxy            bool `json:"is_proxy"`
				IsAnonymous        bool `json:"is_anonymous"`
				IsThreat           bool `json:"is_threat"`
				IsKnownAttacker    bool `json:"is_known_attacker"`
				IsKnownAbuser      bool `json:"is_known_abuser"`
				IsBogon            bool `json:"is_bogon"`
				IsTor              bool `json:"is_tor"`
			} `json:"threat"`
		}

		if err := fetchJSON(url, &resp); err != nil {
			return nil, err
		}

		asn := parseASN(resp.ASN.ASN)

		return &IPInfo{
			IP:           ipAddr,
			IsDatacenter: resp.Threat.IsDatacenter || resp.ASN.Type == "hosting",
			IsProxy:      resp.Threat.IsProxy || resp.Threat.IsAnonymous,
			IsTor:        resp.Threat.IsTor,
			ASN:          asn,
			ASNOrg:       resp.ASN.Name,
			ISP:          resp.ASN.Name,
			Country:      resp.Country,
			CountryCode:  resp.CountryCode,
			City:         resp.City,
			Source:       "ipdata",
		}, nil
	}
}

// ipinfo.io — needs token, 50k/month free
func queryIPInfo(ip string, token string) func(string) (*IPInfo, error) {
	return func(ipAddr string) (*IPInfo, error) {
		url := fmt.Sprintf("https://ipinfo.io/%s?token=%s", ipAddr, token)

		var resp struct {
			City    string `json:"city"`
			Country string `json:"country"`
			Org     string `json:"org"`
			Privacy struct {
				VPN     bool `json:"vpn"`
				Proxy   bool `json:"proxy"`
				Tor     bool `json:"tor"`
				Relay   bool `json:"relay"`
				Hosting bool `json:"hosting"`
			} `json:"privacy"`
		}

		if err := fetchJSON(url, &resp); err != nil {
			return nil, err
		}

		asn := parseASN(resp.Org)

		return &IPInfo{
			IP:           ipAddr,
			IsDatacenter: resp.Privacy.Hosting,
			IsProxy:      resp.Privacy.Proxy || resp.Privacy.Relay,
			IsVPN:        resp.Privacy.VPN,
			IsTor:        resp.Privacy.Tor,
			ASN:          asn,
			ASNOrg:       resp.Org,
			ISP:          resp.Org,
			Country:      resp.Country,
			City:         resp.City,
			Source:       "ipinfo",
		}, nil
	}
}

// parseASN extracts ASN number from strings like "AS16509 Amazon.com, Inc."
func parseASN(s string) int {
	if len(s) < 3 {
		return 0
	}
	// Strip "AS" prefix
	num := s
	if len(s) > 2 && (s[:2] == "AS" || s[:2] == "as") {
		num = s[2:]
	}
	// Take only digits
	end := 0
	for end < len(num) && num[end] >= '0' && num[end] <= '9' {
		end++
	}
	if end == 0 {
		return 0
	}
	result := 0
	for i := 0; i < end; i++ {
		result = result*10 + int(num[i]-'0')
	}
	return result
}

// InitProviders builds the provider chain based on config.
func InitProviders(cfg *Config) []*Provider {
	providers := []*Provider{
		{
			Name:      "ip-api",
			QueryFn:   queryIPAPI,
			RateLimit: 40, // safe margin under 45/min
			NeedsKey:  false,
			HasKey:    true,
		},
		{
			Name:      "ipwhois",
			QueryFn:   queryIPWhois,
			RateLimit: 40, // safe margin under 45/min
			NeedsKey:  false,
			HasKey:    true,
		},
		{
			Name:      "freeipapi",
			QueryFn:   queryFreeIPAPI,
			RateLimit: 55, // safe margin under 60/min
			NeedsKey:  false,
			HasKey:    true,
		},
		{
			Name:      "ipapi-co",
			QueryFn:   queryIPAPICo,
			RateLimit: 25,
			NeedsKey:  false,
			HasKey:    true,
		},
	}

	if cfg.IPDataAPIKey != "" {
		providers = append(providers, &Provider{
			Name:      "ipdata",
			QueryFn:   queryIPData("", cfg.IPDataAPIKey),
			RateLimit: 0, // key-based
			NeedsKey:  true,
			HasKey:    true,
		})
	} else {
		providers = append(providers, &Provider{
			Name:     "ipdata",
			NeedsKey: true,
			HasKey:   false,
		})
	}

	if cfg.IPInfoToken != "" {
		providers = append(providers, &Provider{
			Name:      "ipinfo",
			QueryFn:   queryIPInfo("", cfg.IPInfoToken),
			RateLimit: 0, // key-based
			NeedsKey:  true,
			HasKey:    true,
		})
	} else {
		providers = append(providers, &Provider{
			Name:     "ipinfo",
			NeedsKey: true,
			HasKey:   false,
		})
	}

	// If user specified preferred providers, reorder
	if len(cfg.EnabledProviders) > 0 {
		reordered := make([]*Provider, 0, len(providers))
		provMap := make(map[string]*Provider)
		for _, p := range providers {
			provMap[p.Name] = p
		}
		// Preferred first
		for _, name := range cfg.EnabledProviders {
			if p, ok := provMap[name]; ok {
				reordered = append(reordered, p)
				delete(provMap, name)
			}
		}
		// Rest as fallback
		for _, p := range providers {
			if _, ok := provMap[p.Name]; ok {
				reordered = append(reordered, p)
			}
		}
		providers = reordered
	}

	log.Printf("[providers] Initialized %d providers", len(providers))
	for _, p := range providers {
		status := "ready"
		if p.NeedsKey && !p.HasKey {
			status = "no key"
		}
		log.Printf("[providers]   %s (rate_limit=%d/min, %s)", p.Name, p.RateLimit, status)
	}

	return providers
}
