package lookup

import (
	"log"
	"net"

	"github.com/akl7777777/ip-intel/internal/model"
)

// Version the strict policy to prevent reuse of legacy or incompatible verdicts.
const strictCachePrefix = "s1:"

func strictCacheKey(ip string) string {
	// Canonical IPv6 is at most 39 characters, so the key fits MySQL's VARCHAR(45),
	// including for mixed IPv6/IPv4 input that can otherwise use all 45 characters.
	if parsed := net.ParseIP(ip); parsed != nil {
		ip = parsed.String()
	}
	return strictCachePrefix + ip
}

func (s *Service) lookupStrict(ip string) *model.IPInfo {
	cacheKey := strictCacheKey(ip)
	if info, ok := s.cache.Get(cacheKey); ok {
		info.IP = ip
		return info
	}

	result := &model.IPInfo{IP: ip, Source: "strict-free"}
	if s.localDB != nil {
		if local, err := s.localDB.Lookup(ip); err == nil {
			mergeStrictDetails(result, local)
			if local.IsDatacenter {
				result.IsDatacenter = true
				return s.strictRisk(cacheKey, result, "local")
			}
		}
	}

	if s.store != nil {
		if stored, ok := s.store.Get(cacheKey); ok &&
			(stored.StrictStatus == "all_clear" || stored.StrictStatus == "risk") {
			stored.IP = ip
			stored.Cached = true
			s.cache.Set(cacheKey, stored)
			return stored
		}
	}

	checked := 0
	for _, p := range s.providers {
		if p.NeedsKey {
			continue
		}
		checked++
		if p.StrictQueryFn == nil || !p.Available() {
			log.Printf("[strict] %s unavailable for %s", p.Name, ip)
			return strictUnavailable(result, p.Name)
		}

		p.RecordCall()
		info, err := p.StrictQueryFn(ip)
		if err != nil || info == nil || !sameIP(info.IP, ip) {
			log.Printf("[strict] %s failed for %s: %v", p.Name, ip, err)
			return strictUnavailable(result, p.Name)
		}

		if _, ok := IsKnownDatacenterASN(info.ASN); ok {
			info.IsDatacenter = true
		}
		// Unlike the legacy policy, a residential ASN cannot override a provider's risk.
		mergeStrictDetails(result, info)
		result.IsDatacenter = result.IsDatacenter || info.IsDatacenter
		result.IsProxy = result.IsProxy || info.IsProxy
		result.IsVPN = result.IsVPN || info.IsVPN
		result.IsTor = result.IsTor || info.IsTor
		if result.IsDatacenter || result.IsProxy || result.IsVPN || result.IsTor {
			return s.strictRisk(cacheKey, result, p.Name)
		}
	}

	if checked == 0 {
		return strictUnavailable(result, "none")
	}
	result.StrictStatus = "all_clear"
	s.cache.Set(cacheKey, result)
	s.persistResult(cacheKey, result)
	log.Printf("[strict] %s all %d free providers clear", ip, checked)
	return result
}

func (s *Service) strictRisk(cacheKey string, info *model.IPInfo, provider string) *model.IPInfo {
	info.IsProxy = true
	info.StrictStatus = "risk"
	info.StrictProvider = provider
	s.cache.Set(cacheKey, info)
	s.persistResult(cacheKey, info)
	log.Printf("[strict] %s risk from %s", info.IP, provider)
	return info
}

func strictUnavailable(info *model.IPInfo, provider string) *model.IPInfo {
	// Fail closed for this request, but retry transient failures on the next lookup
	// instead of caching uncertainty for the normal 6 hours / 90 days.
	info.IsProxy = true
	info.StrictStatus = "unavailable"
	info.StrictProvider = provider
	return info
}

func mergeStrictDetails(dst, src *model.IPInfo) {
	if dst.ASN == 0 {
		dst.ASN = src.ASN
	}
	if dst.ASNOrg == "" {
		dst.ASNOrg = src.ASNOrg
	}
	if dst.ISP == "" {
		dst.ISP = src.ISP
	}
	if dst.Country == "" {
		dst.Country = src.Country
	}
	if dst.CountryCode == "" {
		dst.CountryCode = src.CountryCode
	}
	if dst.City == "" {
		dst.City = src.City
	}
}
