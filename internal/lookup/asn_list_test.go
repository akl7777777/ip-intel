package lookup

import "testing"

func TestKnownDatacenterASNsIncludeObservedHostingProviders(t *testing.T) {
	cases := map[int]string{
		142594: "SpeedyPage Ltd",
		199524: "G-Core Labs",
	}

	for asn, wantOrg := range cases {
		gotOrg, ok := IsKnownDatacenterASN(asn)
		if !ok {
			t.Fatalf("ASN %d should be classified as datacenter", asn)
		}
		if gotOrg != wantOrg {
			t.Fatalf("ASN %d org = %q, want %q", asn, gotOrg, wantOrg)
		}
	}
}

func TestKnownResidentialASNsKeepChineseCarriersResidential(t *testing.T) {
	cases := map[int]string{
		4134: "China Telecom (ChinaNet)",
		4812: "China Telecom (Next Carrier Network)",
		9808: "China Mobile",
		4837: "China Unicom (CNCNET)",
	}

	for asn, wantOrg := range cases {
		gotOrg, ok := IsKnownResidentialASN(asn)
		if !ok {
			t.Fatalf("ASN %d should be classified as residential", asn)
		}
		if gotOrg != wantOrg {
			t.Fatalf("ASN %d org = %q, want %q", asn, gotOrg, wantOrg)
		}
	}
}
