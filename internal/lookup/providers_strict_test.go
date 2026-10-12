package lookup

import (
	"io"
	"net/http"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/akl7777777/ip-intel/internal/config"
	"github.com/akl7777777/ip-intel/internal/model"
)

type strictRoundTrip func(*http.Request) (*http.Response, error)

func (f strictRoundTrip) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestStrictProviderResponseValidation(t *testing.T) {
	const ip = "8.8.8.8"
	for _, tc := range []struct {
		name    string
		query   func(string) (*model.IPInfo, error)
		valid   string
		invalid string
	}{
		{"ip-api", queryStrictIPAPI,
			`{"status":"success","query":"8.8.8.8","hosting":false,"proxy":false}`,
			`{"status":"success","query":"8.8.8.8","hosting":false}`},
		{"ipwhois", queryStrictIPWhois,
			`{"success":true,"ip":"8.8.8.8","security":{"anonymous":false,"proxy":false,"vpn":false,"tor":false,"hosting":false}}`,
			`{"success":false,"ip":"8.8.8.8","security":{"anonymous":false,"proxy":false,"vpn":false,"tor":false,"hosting":false}}`},
		{"freeipapi", queryStrictFreeIPAPI,
			`{"ipAddress":"8.8.8.8","countryCode":"US","isProxy":false}`,
			`{"ipAddress":"8.8.8.8","countryCode":"US"}`},
		{"ipapi-co", queryStrictIPAPICo,
			`{"ip":"8.8.8.8","country_code":"US","asn":"AS15169"}`,
			`{"ip":"8.8.8.8","error":true,"country_code":"US","asn":"AS15169"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, response := range []struct {
				body      string
				wantError bool
			}{
				{tc.valid, false},
				{tc.invalid, true},
				{`{}`, true},
				{`{"success":true,"ip":"8.8.8.8","asn":"AS15169"}`, true},
				{strings.ReplaceAll(tc.valid, "8.8.8.8", "1.1.1.1"), true},
			} {
				oldClient := httpClient
				httpClient = &http.Client{Transport: strictRoundTrip(func(*http.Request) (*http.Response, error) {
					return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(response.body))}, nil
				})}
				info, err := tc.query(ip)
				httpClient = oldClient
				if (err != nil) != response.wantError || (!response.wantError && (info == nil || info.IP != ip)) {
					t.Fatalf("response %s: info=%+v, err=%v, wantError=%v", response.body, info, err, response.wantError)
				}
			}
		})
	}
}

func TestConfiguredServiceUsesStrictProviderAdapters(t *testing.T) {
	responses := map[string]string{
		"ip-api.com":    `{"status":"success","query":"8.8.8.8","as":"AS4134","hosting":false,"proxy":false}`,
		"ipwhois.app":   `{"success":true,"ip":"8.8.8.8","asn":"AS4134","security":{"anonymous":false,"proxy":false,"vpn":false,"tor":false,"hosting":false}}`,
		"freeipapi.com": `{"ipAddress":"8.8.8.8","countryCode":"US","isProxy":false}`,
		"ipapi.co":      `{"ip":"8.8.8.8","country_code":"US","asn":"AS4134"}`,
	}
	for _, strict := range []bool{false, true} {
		name := "legacy"
		if strict {
			name = "strict"
		}
		t.Run(name, func(t *testing.T) {
			var called []string
			oldClient := httpClient
			t.Cleanup(func() { httpClient = oldClient })
			httpClient = &http.Client{Transport: strictRoundTrip(func(req *http.Request) (*http.Response, error) {
				called = append(called, req.URL.Host)
				body, ok := responses[req.URL.Host]
				if !ok {
					t.Fatalf("unexpected keyed provider: %s", req.URL.Host)
				}
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, nil
			})}
			svc := NewService(&config.Config{
				CacheTTL:            time.Hour,
				MMDBPath:            filepath.Join(t.TempDir(), "missing.mmdb"),
				IPDataAPIKey:        "test-key",
				IPInfoToken:         "test-token",
				EnabledProviders:    []string{"freeipapi", "ipdata", "ipinfo", "ipwhois", "ip-api", "ipapi-co"},
				StrictFreeProviders: strict,
			})
			defer svc.Close()
			info, err := svc.Lookup("8.8.8.8")
			wantCalls := []string{"freeipapi.com"}
			wantStatus := ""
			if strict {
				wantCalls = []string{"freeipapi.com", "ipwhois.app", "ip-api.com", "ipapi.co"}
				wantStatus = "all_clear"
			}
			if err != nil || info.IsProxy || info.StrictStatus != wantStatus || !reflect.DeepEqual(called, wantCalls) {
				t.Fatalf("configured result=%+v, err=%v, calls=%v, wantCalls=%v", info, err, called, wantCalls)
			}
			if svc.Stats().StrictFreeProviders != strict {
				t.Fatal("stats does not reflect the active policy")
			}
		})
	}
}
