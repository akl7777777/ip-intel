package main

import (
	"log"
)

// Service is the core IP intelligence lookup service.
type Service struct {
	cache     *Cache
	localDB   *LocalDB
	providers []*Provider
}

// NewService creates a new service instance.
func NewService(cfg *Config) *Service {
	return &Service{
		cache:     NewCache(cfg.CacheTTL),
		localDB:   NewLocalDB(cfg.MMDBPath),
		providers: InitProviders(cfg),
	}
}

// Lookup performs an IP intelligence lookup.
// Order: cache → local MMDB + ASN list → external API chain.
func (s *Service) Lookup(ip string) (*IPInfo, error) {
	// 1. Check cache
	if info, ok := s.cache.Get(ip); ok {
		return info, nil
	}

	// 2. Try local MMDB + datacenter ASN list
	if s.localDB != nil {
		info, err := s.localDB.Lookup(ip)
		if err == nil && info.IsDatacenter {
			// Definitively a datacenter IP, no need for API
			s.cache.Set(ip, info)
			log.Printf("[lookup] %s → local (datacenter: ASN %d %s)", ip, info.ASN, info.ASNOrg)
			return info, nil
		}
		// MMDB gave us ASN info but not conclusive about datacenter
		// Continue to API for proxy/VPN detection
		if err == nil {
			// We have partial local info, try to enrich via API
			enriched := s.queryProviders(ip)
			if enriched != nil {
				// Merge: keep API's proxy/vpn/datacenter flags, fill in ASN from local if API missed it
				if enriched.ASN == 0 {
					enriched.ASN = info.ASN
					enriched.ASNOrg = info.ASNOrg
				}
				s.cache.Set(ip, enriched)
				return enriched, nil
			}
			// All APIs failed, return local result
			s.cache.Set(ip, info)
			return info, nil
		}
	}

	// 3. No local DB, go directly to API chain
	info := s.queryProviders(ip)
	if info != nil {
		// Cross-check with ASN list
		if _, ok := IsKnownDatacenterASN(info.ASN); ok {
			info.IsDatacenter = true
		}
		s.cache.Set(ip, info)
		return info, nil
	}

	// 4. All providers failed, return minimal info
	fallback := &IPInfo{
		IP:     ip,
		Source: "none",
	}
	return fallback, nil
}

// queryProviders tries each provider in order until one succeeds.
func (s *Service) queryProviders(ip string) *IPInfo {
	for _, p := range s.providers {
		if !p.Available() {
			continue
		}

		p.RecordCall()
		info, err := p.QueryFn(ip)
		if err != nil {
			log.Printf("[provider] %s failed for %s: %v", p.Name, ip, err)
			continue
		}

		log.Printf("[lookup] %s → %s (datacenter=%v proxy=%v vpn=%v)",
			ip, p.Name, info.IsDatacenter, info.IsProxy, info.IsVPN)
		return info
	}

	log.Printf("[lookup] %s → all providers exhausted", ip)
	return nil
}

// Stats returns service statistics.
func (s *Service) Stats() *StatsResponse {
	providerStatuses := make([]ProviderStatus, len(s.providers))
	for i, p := range s.providers {
		providerStatuses[i] = ProviderStatus{
			Name:        p.Name,
			Available:   p.Available(),
			RateLimit:   p.RateLimit,
			UsedLastMin: p.UsedLastMinute(),
			NeedsKey:    p.NeedsKey,
			HasKey:      p.HasKey,
		}
	}

	return &StatsResponse{
		CacheSize:  s.cache.Size(),
		CacheTTL:   s.cache.ttl.String(),
		Providers:  providerStatuses,
		LocalDB:    s.localDB != nil,
		KnownASNs:  len(datacenterASNs),
	}
}

// Close cleans up resources.
func (s *Service) Close() {
	s.cache.Stop()
	s.localDB.Close()
}
