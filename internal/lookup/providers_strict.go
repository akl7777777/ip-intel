package lookup

import (
	"fmt"
	"net"

	"github.com/akl7777777/ip-intel/internal/model"
)

// These adapters intentionally leave the legacy parsers unchanged. Pointer booleans
// distinguish an explicit safe signal from a field that the free tier did not return.

func sameIP(a, b string) bool {
	first, second := net.ParseIP(a), net.ParseIP(b)
	return first != nil && second != nil && first.Equal(second)
}

func queryStrictIPAPI(ip string) (*model.IPInfo, error) {
	url := fmt.Sprintf("http://ip-api.com/json/%s?fields=status,message,query,country,countryCode,city,isp,org,as,hosting,proxy", ip)
	var resp struct {
		Status      string `json:"status"`
		Query       string `json:"query"`
		Country     string `json:"country"`
		CountryCode string `json:"countryCode"`
		City        string `json:"city"`
		ISP         string `json:"isp"`
		Org         string `json:"org"`
		AS          string `json:"as"`
		Hosting     *bool  `json:"hosting"`
		Proxy       *bool  `json:"proxy"`
	}
	if err := fetchJSON(url, &resp); err != nil {
		return nil, err
	}
	if resp.Status != "success" || !sameIP(resp.Query, ip) || resp.Hosting == nil || resp.Proxy == nil {
		return nil, fmt.Errorf("ip-api incomplete or unsuccessful response")
	}
	return &model.IPInfo{
		IP: ip, IsDatacenter: *resp.Hosting, IsProxy: *resp.Proxy,
		ASN: parseASN(resp.AS), ASNOrg: resp.Org, ISP: resp.ISP,
		Country: resp.Country, CountryCode: resp.CountryCode, City: resp.City,
		Source: "ip-api",
	}, nil
}

func queryStrictIPWhois(ip string) (*model.IPInfo, error) {
	url := fmt.Sprintf("https://ipwhois.app/json/%s?security=1", ip)
	var resp struct {
		Success     *bool  `json:"success"`
		IP          string `json:"ip"`
		Country     string `json:"country"`
		CountryCode string `json:"country_code"`
		City        string `json:"city"`
		ISP         string `json:"isp"`
		Org         string `json:"org"`
		ASN         string `json:"asn"`
		Security    *struct {
			Anonymous *bool `json:"anonymous"`
			Proxy     *bool `json:"proxy"`
			VPN       *bool `json:"vpn"`
			Tor       *bool `json:"tor"`
			Hosting   *bool `json:"hosting"`
		} `json:"security"`
	}
	if err := fetchJSON(url, &resp); err != nil {
		return nil, err
	}
	security := resp.Security
	if resp.Success == nil || !*resp.Success || !sameIP(resp.IP, ip) || security == nil ||
		security.Anonymous == nil || security.Proxy == nil || security.VPN == nil ||
		security.Tor == nil || security.Hosting == nil {
		return nil, fmt.Errorf("ipwhois incomplete or unsuccessful response")
	}
	return &model.IPInfo{
		IP: ip, IsDatacenter: *security.Hosting,
		IsProxy: *security.Proxy || *security.Anonymous,
		IsVPN:   *security.VPN, IsTor: *security.Tor,
		ASN: parseASN(resp.ASN), ASNOrg: resp.Org, ISP: resp.ISP,
		Country: resp.Country, CountryCode: resp.CountryCode, City: resp.City,
		Source: "ipwhois",
	}, nil
}

func queryStrictFreeIPAPI(ip string) (*model.IPInfo, error) {
	url := fmt.Sprintf("https://freeipapi.com/api/json/%s", ip)
	var resp struct {
		IPAddress   string `json:"ipAddress"`
		CountryName string `json:"countryName"`
		CountryCode string `json:"countryCode"`
		CityName    string `json:"cityName"`
		IsProxy     *bool  `json:"isProxy"`
	}
	if err := fetchJSON(url, &resp); err != nil {
		return nil, err
	}
	if !sameIP(resp.IPAddress, ip) || resp.CountryCode == "" || resp.IsProxy == nil {
		return nil, fmt.Errorf("freeipapi incomplete response")
	}
	return &model.IPInfo{
		IP: ip, IsProxy: *resp.IsProxy,
		Country: resp.CountryName, CountryCode: resp.CountryCode, City: resp.CityName,
		Source: "freeipapi",
	}, nil
}

func queryStrictIPAPICo(ip string) (*model.IPInfo, error) {
	url := fmt.Sprintf("https://ipapi.co/%s/json/", ip)
	var resp struct {
		IP          string `json:"ip"`
		Error       bool   `json:"error"`
		Country     string `json:"country_name"`
		CountryCode string `json:"country_code"`
		City        string `json:"city"`
		Org         string `json:"org"`
		ASN         string `json:"asn"`
	}
	if err := fetchJSON(url, &resp); err != nil {
		return nil, err
	}
	asn := parseASN(resp.ASN)
	if resp.Error || !sameIP(resp.IP, ip) || resp.CountryCode == "" || asn == 0 {
		return nil, fmt.Errorf("ipapi.co incomplete or unsuccessful response")
	}
	info := &model.IPInfo{
		IP: ip, ASN: asn, ASNOrg: resp.Org, ISP: resp.Org,
		Country: resp.Country, CountryCode: resp.CountryCode, City: resp.City,
		Source: "ipapi-co",
	}
	if _, ok := IsKnownDatacenterASN(asn); ok {
		info.IsDatacenter = true
	}
	return info, nil
}
