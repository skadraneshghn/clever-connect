package desync

import (
	"strconv"
	"strings"
)

type EvasionConfig struct {
	Enabled      bool
	FakeTTL      uint8
	InjectBadSum bool
	InjectBadSeq bool
	FragmentSize int
	TargetPorts  []int
}

// ParseCLIArgs parses ByeByeDPI style arguments (e.g., "-d1+s -s29+s -t 5")
func ParseCLIArgs(args string) *EvasionConfig {
	config := &EvasionConfig{
		Enabled:     true,
		FakeTTL:     8,
		TargetPorts: []int{443, 80}, // Default targeted ports
	}

	parts := strings.Split(args, " ")
	for i, p := range parts {
		if strings.HasPrefix(p, "-t") { // Fake TTL setting
			// Logic to extract integer after -t (e.g. TTL 5)
			ttlStr := strings.TrimPrefix(p, "-t")
			if ttlStr == "" && i+1 < len(parts) {
				ttlStr = parts[i+1]
			}
			if ttl, err := strconv.Atoi(ttlStr); err == nil {
				config.FakeTTL = uint8(ttl)
			}
		} else if strings.Contains(p, "+s") {
			// Trigger BadSeq/BadSum logic based on ByeByeDPI syntax
			config.InjectBadSum = true
			config.InjectBadSeq = true
		}
		// Add parsing for fragmentation (-f) and disorder (-d)
		if strings.HasPrefix(p, "-f") {
			fragStr := strings.TrimPrefix(p, "-f")
			if fragStr == "" && i+1 < len(parts) {
				fragStr = parts[i+1]
			}
			if fragSize, err := strconv.Atoi(fragStr); err == nil {
				config.FragmentSize = fragSize
			}
		}
	}
	return config
}
