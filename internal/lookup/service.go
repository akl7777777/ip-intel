package lookup

import (
	"log"

	"github.com/akl7777777/ip-intel/internal/cache"
	"github.com/akl7777777/ip-intel/internal/config"
	"github.com/akl7777777/ip-intel/internal/model"
	"github.com/akl7777777/ip-intel/internal/store"
)

// Service is the core IP intelligence lookup service.
type Service struct {
	cache     *cache.Cache
	store     store.Store // persistent cache (SQLite/MySQL), may be nil
	localDB   *LocalDB
	providers []*Provider
}

// NewService creates a new service instance.
func NewService(cfg *config.Config) *Service {
	svc := &Service{
		cache:     cache.New(cfg.CacheTTL),
		localDB:   NewLocalDB(cfg.MMDBPath),
		providers: InitProviders(cfg),
	}

	if cfg.PersistentCache {
		s, err := store.New(cfg.PersistentCacheType, cfg.PersistentCacheDSN, cfg.PersistentCacheTTL)
		if err != nil {
			log.Printf("[store] WARNING: Failed to open persistent cache: %v", err)
		} else {
			svc.store = s
		}
	}

	return svc
}

// Lookup performs an IP intelligence lookup.
// Order: cache → local MMDB + ASN list → persistent cache → external API chain.
func (s *Service) Lookup(ip string) (*model.IPInfo, error) {
	// 1. Check in-memory cache
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
		// Continue to persistent cache / API for proxy/VPN detection
		if err == nil {
			// 3. Check persistent cache before hitting external APIs
			if s.store != nil {
				if stored, ok := s.store.Get(ip); ok {
					// Merge local ASN info if persistent cache missed it
					if stored.ASN == 0 {
						stored.ASN = info.ASN
						stored.ASNOrg = info.ASNOrg
					}
					stored.Cached = true
					s.cache.Set(ip, stored)
					log.Printf("[lookup] %s → persistent cache (source=%s)", ip, stored.Source)
					return stored, nil
				}
			}

			// 4. Try external API for enrichment
			enriched := s.queryProviders(ip)
			if enriched != nil {
				// Merge: keep API's proxy/vpn/datacenter flags, fill in ASN from local if API missed it
				if enriched.ASN == 0 {
					enriched.ASN = info.ASN
					enriched.ASNOrg = info.ASNOrg
				}
				s.cache.Set(ip, enriched)
				s.persistResult(ip, enriched)
				return enriched, nil
			}
			// All APIs failed, return local result
			s.cache.Set(ip, info)
			return info, nil
		}
	}

	// 3b. No local DB — check persistent cache
	if s.store != nil {
		if stored, ok := s.store.Get(ip); ok {
			stored.Cached = true
			s.cache.Set(ip, stored)
			log.Printf("[lookup] %s → persistent cache (source=%s)", ip, stored.Source)
			return stored, nil
		}
	}

	// 5. No local DB, go directly to API chain
	info := s.queryProviders(ip)
	if info != nil {
		// Cross-check with ASN list
		if _, ok := IsKnownDatacenterASN(info.ASN); ok {
			info.IsDatacenter = true
		}
		s.cache.Set(ip, info)
		s.persistResult(ip, info)
		return info, nil
	}

	// 6. All providers failed, return minimal info
	fallback := &model.IPInfo{
		IP:     ip,
		Source: "none",
	}
	return fallback, nil
}

// persistResult saves the lookup result to persistent cache if enabled.
func (s *Service) persistResult(ip string, info *model.IPInfo) {
	if s.store != nil {
		s.store.Set(ip, info)
	}
}

// queryProviders tries each provider in order until one succeeds.
func (s *Service) queryProviders(ip string) *model.IPInfo {
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
func (s *Service) Stats() *model.StatsResponse {
	providerStatuses := make([]model.ProviderStatus, len(s.providers))
	for i, p := range s.providers {
		providerStatuses[i] = model.ProviderStatus{
			Name:        p.Name,
			Available:   p.Available(),
			RateLimit:   p.RateLimit,
			UsedLastMin: p.UsedLastMinute(),
			NeedsKey:    p.NeedsKey,
			HasKey:      p.HasKey,
		}
	}

	resp := &model.StatsResponse{
		CacheSize:              s.cache.Size(),
		CacheTTL:               s.cache.TTL().String(),
		PersistentCacheEnabled: s.store != nil,
		Providers:              providerStatuses,
		LocalDB:                s.localDB != nil,
		KnownASNs:              len(DatacenterASNs),
	}

	if s.store != nil {
		resp.PersistentCacheSize = s.store.Size()
	}

	return resp
}

// Close cleans up resources.
func (s *Service) Close() {
	s.cache.Stop()
	s.localDB.Close()
	if s.store != nil {
		s.store.Close()
	}
}
