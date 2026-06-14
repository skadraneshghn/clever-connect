package scratch

import (
	"fmt"
	"net"
	"os"
	"path/filepath"

	"github.com/oschwald/geoip2-golang"
)

func runTestNali() {
	dir := "data/geo"
	cityPath := filepath.Join(dir, "GeoLite2-City.mmdb")
	asnPath := filepath.Join(dir, "GeoLite2-ASN.mmdb")

	if _, err := os.Stat(cityPath); os.IsNotExist(err) {
		fmt.Println("GeoLite2-City.mmdb does not exist")
		return
	}

	cityDB, err := geoip2.Open(cityPath)
	if err != nil {
		fmt.Printf("Open City DB error: %v\n", err)
		return
	}
	defer cityDB.Close()

	asnDB, err := geoip2.Open(asnPath)
	if err != nil {
		fmt.Printf("Open ASN DB error: %v\n", err)
		return
	}
	defer asnDB.Close()

	ips := []string{"1.1.1.1", "8.8.8.8", "2606:4700:4700::1111", "2001:4860:4860::8888"}
	for _, ipStr := range ips {
		ip := net.ParseIP(ipStr)
		if ip == nil {
			fmt.Printf("Invalid IP: %s\n", ipStr)
			continue
		}

		cityRecord, err := cityDB.City(ip)
		countryName := "Unknown"
		cityName := "Unknown"
		if err == nil && cityRecord != nil {
			countryName = cityRecord.Country.Names["en"]
			cityName = cityRecord.City.Names["en"]
		}

		asnRecord, err := asnDB.ASN(ip)
		asnOrg := "Unknown"
		asnNum := uint(0)
		if err == nil && asnRecord != nil {
			asnOrg = asnRecord.AutonomousSystemOrganization
			asnNum = asnRecord.AutonomousSystemNumber
		}

		fmt.Printf("IP: %s | Country: %s | City: %s | ASN: AS%d (%s)\n", ipStr, countryName, cityName, asnNum, asnOrg)
	}
}
