package lookup

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/akl7777777/ip-intel/internal/cache"
	"github.com/akl7777777/ip-intel/internal/model"
	"github.com/akl7777777/ip-intel/internal/store"
)

type memoryStore struct {
	items map[string]*model.IPInfo
}

func (s *memoryStore) Get(ip string) (*model.IPInfo, bool) {
	info, ok := s.items[ip]
	if !ok {
		return nil, false
	}
	copy := *info
	return &copy, true
}

func (s *memoryStore) Set(ip string, info *model.IPInfo) {
	copy := *info
	s.items[ip] = &copy
}

func (s *memoryStore) Size() int { return len(s.items) }
func (s *memoryStore) Cleanup()  {}
func (s *memoryStore) Close()    {}

func testService(strict bool, providers ...*Provider) *Service {
	return &Service{
		cache:               cache.New(time.Hour),
		store:               &memoryStore{items: make(map[string]*model.IPInfo)},
		providers:           providers,
		strictFreeProviders: strict,
	}
}

func freeProvider(name string, fn func(string) (*model.IPInfo, error)) *Provider {
	return &Provider{Name: name, StrictQueryFn: fn, RateLimit: 100, HasKey: true}
}

func clearInfo(ip string) (*model.IPInfo, error) {
	return &model.IPInfo{IP: ip, CountryCode: "US"}, nil
}

func TestStrictRequiresEveryFreeProvider(t *testing.T) {
	called := []string{}
	providers := []*Provider{}
	for _, name := range []string{"ip-api", "ipwhois", "freeipapi", "ipapi-co"} {
		name := name
		providers = append(providers, freeProvider(name, func(ip string) (*model.IPInfo, error) {
			called = append(called, name)
			return clearInfo(ip)
		}))
	}
	providers = append(providers, &Provider{Name: "ipdata", NeedsKey: true, HasKey: true,
		StrictQueryFn: func(string) (*model.IPInfo, error) {
			t.Fatal("keyed provider should not be queried")
			return nil, nil
		},
	})
	svc := testService(true, providers...)
	defer svc.cache.Stop()

	info, err := svc.Lookup("8.8.8.8")
	if err != nil || info.IsProxy || info.StrictStatus != "all_clear" || info.Source != "strict-free" {
		t.Fatalf("strict clear result = %+v, err=%v", info, err)
	}
	if len(called) != 4 {
		t.Fatalf("called %d free providers, want 4: %v", len(called), called)
	}
	if _, ok := svc.store.Get("8.8.8.8"); ok {
		t.Fatal("strict result wrote to legacy cache key")
	}
	if _, ok := svc.store.Get(strictCachePrefix + "8.8.8.8"); !ok {
		t.Fatal("strict result was not persisted under its own key")
	}
	second, err := svc.Lookup("8.8.8.8")
	if err != nil || !second.Cached || len(called) != 4 {
		t.Fatalf("strict cache result = %+v, err=%v, calls=%v", second, err, called)
	}
}

func TestStrictStopsAtFirstRisk(t *testing.T) {
	for _, tc := range []struct {
		name string
		flag func(*model.IPInfo)
	}{
		{"proxy", func(info *model.IPInfo) { info.IsProxy = true }},
		{"vpn", func(info *model.IPInfo) { info.IsVPN = true }},
		{"tor", func(info *model.IPInfo) { info.IsTor = true }},
		{"datacenter", func(info *model.IPInfo) { info.IsDatacenter = true }},
		{"residential ASN cannot override risk", func(info *model.IPInfo) {
			info.ASN = 4134
			info.IsDatacenter = true
		}},
		{"known ASN", func(info *model.IPInfo) { info.ASN = 16509 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			svc := testService(true,
				freeProvider("first", func(ip string) (*model.IPInfo, error) {
					calls++
					return clearInfo(ip)
				}),
				freeProvider("second", func(ip string) (*model.IPInfo, error) {
					calls++
					info, _ := clearInfo(ip)
					tc.flag(info)
					return info, nil
				}),
				freeProvider("third", func(ip string) (*model.IPInfo, error) {
					calls++
					return clearInfo(ip)
				}),
			)
			defer svc.cache.Stop()
			info, err := svc.Lookup("8.8.8.8")
			if err != nil || !info.IsProxy || info.StrictStatus != "risk" || info.StrictProvider != "second" || calls != 2 {
				t.Fatalf("risk result = %+v, err=%v, calls=%d", info, err, calls)
			}
		})
	}
}

func TestStrictUnavailableFailsClosedWithoutCaching(t *testing.T) {
	for _, tc := range []struct {
		name     string
		provider *Provider
	}{
		{"error", freeProvider("failed", func(string) (*model.IPInfo, error) { return nil, errors.New("timeout") })},
		{"invalid IP", freeProvider("failed", func(string) (*model.IPInfo, error) {
			return &model.IPInfo{IP: "1.1.1.1"}, nil
		})},
		{"nil result", freeProvider("failed", func(string) (*model.IPInfo, error) { return nil, nil })},
		{"rate limit", &Provider{Name: "failed", StrictQueryFn: clearInfo, RateLimit: 1, HasKey: true, callTimes: []int64{time.Now().Unix()}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := testService(true, tc.provider)
			defer svc.cache.Stop()
			info, err := svc.Lookup("8.8.8.8")
			if err != nil || !info.IsProxy || info.StrictStatus != "unavailable" || info.StrictProvider != "failed" {
				t.Fatalf("unavailable result = %+v, err=%v", info, err)
			}
			if svc.cache.Size() != 0 || svc.store.Size() != 0 {
				t.Fatal("uncertain result must not be cached")
			}
		})
	}
}

func TestStrictFailureCanRecover(t *testing.T) {
	calls := 0
	svc := testService(true, freeProvider("free", func(ip string) (*model.IPInfo, error) {
		calls++
		if calls == 1 {
			return nil, errors.New("temporary failure")
		}
		return clearInfo(ip)
	}))
	defer svc.cache.Stop()
	first, _ := svc.Lookup("8.8.8.8")
	second, _ := svc.Lookup("8.8.8.8")
	if first.StrictStatus != "unavailable" || second.StrictStatus != "all_clear" || second.Cached || calls != 2 {
		t.Fatalf("temporary failure did not recover: first=%+v, second=%+v, calls=%d", first, second, calls)
	}
}

func TestStrictWithoutFreeProvidersFailsClosed(t *testing.T) {
	svc := testService(true, &Provider{Name: "ipdata", NeedsKey: true})
	defer svc.cache.Stop()
	info, _ := svc.Lookup("8.8.8.8")
	if !info.IsProxy || info.StrictStatus != "unavailable" || svc.cache.Size() != 0 {
		t.Fatalf("empty provider chain passed: %+v", info)
	}
}

func TestStrictCacheKeyFitsMySQLIPv6Column(t *testing.T) {
	for _, ip := range []string{
		"8.8.8.8",
		"ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff",
		"ffff:ffff:ffff:ffff:ffff:ffff:255.255.255.255",
	} {
		if key := strictCacheKey(ip); len(key) > 45 || !strings.HasPrefix(key, strictCachePrefix) {
			t.Fatalf("cache key %q is incompatible with MySQL", key)
		}
	}
}

func TestStrictSQLiteCacheSurvivesRestartWithoutAffectingLegacy(t *testing.T) {
	const ip = "2001:db8::1234"
	path := filepath.Join(t.TempDir(), "ip-cache.db")
	db, err := store.NewSQLite(path, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	db.Set(ip, &model.IPInfo{IP: ip, Source: "legacy"})
	svc := testService(true, freeProvider("free", clearInfo))
	svc.store = db
	first, _ := svc.Lookup(ip)
	svc.Close()
	if first.StrictStatus != "all_clear" || first.Cached {
		t.Fatalf("unexpected fresh strict result: %+v", first)
	}

	reopened, err := store.NewSQLite(path, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	svc = testService(true, freeProvider("free", func(string) (*model.IPInfo, error) {
		t.Fatal("strict persisted result should avoid a provider request after restart")
		return nil, nil
	}))
	svc.store = reopened
	defer svc.Close()
	second, _ := svc.Lookup("2001:0db8:0:0:0:0:0:1234")
	if second.StrictStatus != "all_clear" || !second.Cached || second.IP != "2001:0db8:0:0:0:0:0:1234" {
		t.Fatalf("unexpected persisted strict result: %+v", second)
	}
	legacy, ok := reopened.Get(ip)
	if !ok || legacy.Source != "legacy" || legacy.StrictStatus != "" {
		t.Fatalf("legacy entry changed: %+v", legacy)
	}
}

func TestStrictAndLegacyCachesStaySeparate(t *testing.T) {
	const ip = "8.8.8.8"
	shared := &memoryStore{items: map[string]*model.IPInfo{
		ip: {IP: ip, Source: "legacy", IsProxy: false},
	}}
	strict := testService(true, freeProvider("free", clearInfo))
	strict.store = shared
	defer strict.cache.Stop()
	info, err := strict.Lookup(ip)
	if err != nil || info.StrictStatus != "all_clear" || info.Cached {
		t.Fatalf("strict mode reused legacy result: %+v, err=%v", info, err)
	}
	legacy := testService(false)
	legacy.store = shared
	defer legacy.cache.Stop()
	info, err = legacy.Lookup(ip)
	if err != nil || info.Source != "legacy" || info.StrictStatus != "" {
		t.Fatalf("legacy mode reused strict result: %+v, err=%v", info, err)
	}
}

func TestLegacyStillStopsAtFirstSuccess(t *testing.T) {
	calls := 0
	svc := testService(false,
		&Provider{Name: "first", RateLimit: 100, HasKey: true, QueryFn: func(ip string) (*model.IPInfo, error) {
			calls++
			return &model.IPInfo{IP: ip, Source: "first"}, nil
		}},
		&Provider{Name: "second", RateLimit: 100, HasKey: true, QueryFn: func(ip string) (*model.IPInfo, error) {
			calls++
			return &model.IPInfo{IP: ip, Source: "second", IsProxy: true}, nil
		}},
	)
	defer svc.cache.Stop()
	info, err := svc.Lookup("8.8.8.8")
	if err != nil || calls != 1 || info.IsProxy || info.Source != "first" || info.StrictStatus != "" {
		t.Fatalf("legacy result = %+v, err=%v, calls=%d", info, err, calls)
	}
	body, err := json.Marshal(info)
	if err != nil || strings.Contains(string(body), "strict_") {
		t.Fatalf("legacy JSON changed: %s, err=%v", body, err)
	}
}

func TestLegacyFailuresStillFallThrough(t *testing.T) {
	calls := 0
	svc := testService(false,
		&Provider{Name: "failed", RateLimit: 100, HasKey: true, QueryFn: func(string) (*model.IPInfo, error) {
			calls++
			return nil, errors.New("upstream failure")
		}},
		&Provider{Name: "working", RateLimit: 100, HasKey: true, QueryFn: func(ip string) (*model.IPInfo, error) {
			calls++
			return &model.IPInfo{IP: ip, Source: "working", ASN: 4134, IsDatacenter: true}, nil
		}},
	)
	defer svc.cache.Stop()
	info, err := svc.Lookup("8.8.8.8")
	if err != nil || calls != 2 || info.Source != "working" || info.IsDatacenter || info.IsProxy || info.StrictStatus != "" {
		t.Fatalf("legacy fallback or residential correction changed: %+v, err=%v, calls=%d", info, err, calls)
	}
}
