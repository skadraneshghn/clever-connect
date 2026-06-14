package models

import (
	"time"

	"gorm.io/gorm"
)

// DNSResolver represents a DNS server target and its configuration profiles
type DNSResolver struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	// Primary Identity
	IP       string `gorm:"size:100;not null;uniqueIndex:idx_ip_proto" json:"ip"`
	Protocol string `gorm:"size:20;not null;uniqueIndex:idx_ip_proto" json:"protocol"` // udp, tcp, dot, doh, doq

	// Network & Geo Metadata
	ProviderName string `gorm:"size:150;index" json:"provider_name"`
	ASN          string `gorm:"size:50;index" json:"asn"`
	ISP          string `gorm:"size:255" json:"isp"`
	CountryCode  string `gorm:"size:10;index" json:"country_code"`
	CountryName  string `gorm:"size:100" json:"country_name"`

	// Capabilities Toggles
	SupportUDP bool `gorm:"default:true" json:"support_udp"`
	SupportTCP bool `gorm:"default:false" json:"support_tcp"`
	SupportDoT bool `gorm:"default:false" json:"support_dot"`
	SupportDoH bool `gorm:"default:false" json:"support_doh"`
	SupportDoQ bool `gorm:"default:false" json:"support_doq"`

	// Reliability & Security Profiles
	CensorshipStatus string `gorm:"size:50;default:'unverified';index" json:"censorship_status"` // clean, hijacked, manipulated, sinkhole
	DNSSECOverride   bool   `gorm:"default:false" json:"dnssec_override"`                       // DNSSEC stripped
	DNSRebindingVuln bool   `gorm:"default:false" json:"dns_rebinding_vuln"`
	IsCustom         bool   `gorm:"default:false;index" json:"is_custom"`                        // Custom user-added
	Category         string `gorm:"size:100;default:'general';index" json:"category"`           // security, general, regional, family
}

// DNSTesterConfig stores runtime parameters for the testing run
type DNSTesterConfig struct {
	gorm.Model
	ConcurrencyLimit int         `json:"concurrency_limit" gorm:"default:100"`
	QPSLimit         int         `json:"qps_limit" gorm:"default:500"`
	TimeoutMs        int         `json:"timeout_ms" gorm:"default:3000"`
	Attempts         int         `json:"attempts" gorm:"default:3"`
	CacheBusting     bool        `json:"cache_busting" gorm:"default:true"`
	ReferenceDomain  string      `json:"reference_domain" gorm:"default:'google.com'"`
	QueryTypes       StringArray `json:"query_types" gorm:"type:text"` // A, AAAA, MX, TXT, etc.
	DNSClass         string      `json:"dns_class" gorm:"size:20;default:'IN'"`
	QueryGenerator   string      `json:"query_generator" gorm:"size:50;default:'random'"` // static, random, sequential
	DomainSource     string      `json:"domain_source" gorm:"size:50;default:'default'"`   // default, custom, url
	CustomDomains    StringArray `json:"custom_domains" gorm:"type:text"`
	WordlistURL      string      `json:"wordlist_url" gorm:"size:512"`
	ExpectResponse   string      `json:"expect_response" gorm:"size:255"`
}
