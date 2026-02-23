package lookup

// DatacenterASNs contains known datacenter/cloud/hosting provider ASNs.
// Only includes providers that are indisputably hosting infrastructure.
// Source: public BGP data + official provider documentation.
var DatacenterASNs = map[int]string{
	// === Major Cloud Providers ===
	16509:  "Amazon.com / AWS",
	14618:  "Amazon.com / AWS",
	8075:   "Microsoft Azure",
	15169:  "Google Cloud",
	396982: "Google Cloud",
	45102:  "Alibaba Cloud",
	45090:  "Tencent Cloud",
	132203: "Tencent Cloud",
	31898:  "Oracle Cloud",
	36351:  "IBM Cloud / SoftLayer",
	13335:  "Cloudflare",

	// === VPS / Hosting Providers ===
	14061:  "DigitalOcean",
	20473:  "Vultr / Choopa",
	63949:  "Linode / Akamai Connected Cloud",
	396998: "Linode / Akamai Connected Cloud",
	16276:  "OVHcloud",
	24940:  "Hetzner Online",
	12876:  "Scaleway (Online SAS)",
	40021:  "Contabo",
	51167:  "Contabo",
	209605: "Contabo Asia",
	60781:  "LeaseWeb",
	28753:  "LeaseWeb",
	30633:  "LeaseWeb",
	9009:   "M247 / G-Core Labs",
	202053: "UpCloud",
	35540:  "MivoCloud",
	42730:  "EVOCATIVE (eStruxture)",
	55286:  "Equinix Metal (Packet)",
	13414:  "Twitter / X Infrastructure",

	// === Dedicated Server / Colocation ===
	33070:  "Rackspace",
	19994:  "Rackspace",
	36352:  "ColoCrossing",
	40676:  "Psychz Networks",
	8100:   "QuadraNet",
	23352:  "ServerCentral",
	21859:  "Zenlayer",
	54574:  "DMIT",
	906:    "DMIT",
	25820:  "IT7 Networks (BandwagonHost)",
	36007:  "Kamatera",
	54290:  "Hostwinds",
	62567:  "DigitalOcean (NYC)",
	46664:  "VolumeDrive",
	30083:  "HEG US (Hetzner US)",
	62563:  "GTHost",
	398101: "GoDaddy Cloud",
	26496:  "GoDaddy Hosting",
	394695: "Google Cloud (Dedicated)",
	19527:  "Google Fiber (Cloud)",

	// === Asian Hosting Providers ===
	38001:  "NewMedia Express (SG)",
	45753:  "NTT SmartConnect (JP)",
	132335: "LeapSwitch (IN)",
	55720:  "Gigabit Hosting (MY)",
	38731:  "Vietel IDC (VN)",
	45899:  "VNPT (VN IDC)",
	56040:  "China Mobile Cloud",
	37963:  "Alibaba Cloud (HK)",
	58461:  "China Telecom Cloud",
	131477: "Sify Technologies (IN)",
	55933:  "Cloudie (HK)",
	141995: "Tencent Cloud AP",

	// === European Hosting ===
	47583:  "Hostinger",
	44477:  "Stark Industries (Hosting)",
	197540: "Netcup",
	34549:  "Meer Web (NL)",
	29066:  "velia.net",
	50673:  "Serverius",
	60068:  "Datacamp (CDN77)",
	212238: "Datacamp (CDN77)",
	42831:  "UK Dedicated Servers",
	213230: "Hetzner Cloud",
	200019: "AlexHost (MD)",
	59711:  "HZ Hosting",

	// === Russian / CIS Hosting ===
	49981:  "WorldStream",
	48282:  "HIVELOCITY",
	35415:  "Webzilla",
	50979:  "Selectel",
	208091: "Postman (Hosting)",

	// === VPN / Proxy Infrastructure ===
	206092: "VPN providers infrastructure",
	396356: "Maxihost",

	// === CDN / Edge ===
	20940:  "Akamai Technologies",
	54113:  "Fastly",
	209242: "Cloudflare (WARP)",
	132892: "Cloudflare (AP)",
	397213: "Cloudflare",
}

// IsKnownDatacenterASN checks if an ASN belongs to a known datacenter.
func IsKnownDatacenterASN(asn int) (string, bool) {
	org, ok := DatacenterASNs[asn]
	return org, ok
}
