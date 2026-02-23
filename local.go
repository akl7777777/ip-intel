package main

import (
	"fmt"
	"log"
	"net"
	"os"

	"github.com/oschwald/maxminddb-golang"
)

// LocalDB handles MMDB-based local IP lookups.
type LocalDB struct {
	reader *maxminddb.Reader
}

// mmdbRecord maps the fields in a GeoLite2-ASN MMDB.
type mmdbRecord struct {
	AutonomousSystemNumber       int    `maxminddb:"autonomous_system_number"`
	AutonomousSystemOrganization string `maxminddb:"autonomous_system_organization"`
}

// NewLocalDB tries to open the MMDB file. Returns nil if not available.
func NewLocalDB(path string) *LocalDB {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		log.Printf("[local] MMDB file not found at %s, local lookup disabled", path)
		return nil
	}

	reader, err := maxminddb.Open(path)
	if err != nil {
		log.Printf("[local] Failed to open MMDB: %v, local lookup disabled", err)
		return nil
	}

	log.Printf("[local] Loaded MMDB: %s", path)
	return &LocalDB{reader: reader}
}

// Lookup queries the local MMDB for ASN info, then checks the datacenter ASN list.
func (db *LocalDB) Lookup(ipStr string) (*IPInfo, error) {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return nil, fmt.Errorf("invalid IP: %s", ipStr)
	}

	var record mmdbRecord
	err := db.reader.Lookup(ip, &record)
	if err != nil {
		return nil, fmt.Errorf("MMDB lookup failed: %w", err)
	}

	info := &IPInfo{
		IP:          ipStr,
		ASN:         record.AutonomousSystemNumber,
		ASNOrg:      record.AutonomousSystemOrganization,
		ISP:         record.AutonomousSystemOrganization,
		Source:      "local",
	}

	// Check against known datacenter ASN list
	if org, ok := IsKnownDatacenterASN(record.AutonomousSystemNumber); ok {
		info.IsDatacenter = true
		info.ASNOrg = org
	}

	return info, nil
}

// Close closes the MMDB reader.
func (db *LocalDB) Close() {
	if db != nil && db.reader != nil {
		db.reader.Close()
	}
}

// LookupByASNOnly checks the embedded ASN list without MMDB.
// Used when no MMDB is available.
func LookupByASNOnly(asn int) (string, bool) {
	return IsKnownDatacenterASN(asn)
}
