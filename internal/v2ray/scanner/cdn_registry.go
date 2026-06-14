package scanner

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type cdnTrieNode struct {
	children [2]*cdnTrieNode
	value    string
}

type CDNRegistry struct {
	mu     sync.RWMutex
	v4Root *cdnTrieNode
	v6Root *cdnTrieNode
}

var GlobalCDNRegistry = &CDNRegistry{
	v4Root: &cdnTrieNode{},
	v6Root: &cdnTrieNode{},
}

func (t *CDNRegistry) Insert(cidrStr string, value string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	_, ipNet, err := net.ParseCIDR(strings.TrimSpace(cidrStr))
	if err != nil {
		return err
	}

	ones, _ := ipNet.Mask.Size()
	ip := ipNet.IP

	var root *cdnTrieNode
	if ip.To4() != nil {
		root = t.v4Root
		ip = ip.To4()
	} else {
		root = t.v6Root
	}

	curr := root
	for i := 0; i < ones; i++ {
		byteIdx := i / 8
		bitIdx := 7 - (i % 8)
		bit := (ip[byteIdx] >> bitIdx) & 1

		if curr.children[bit] == nil {
			curr.children[bit] = &cdnTrieNode{}
		}
		curr = curr.children[bit]
	}
	curr.value = value
	return nil
}

func (t *CDNRegistry) Lookup(ip net.IP) (string, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var curr *cdnTrieNode
	var bits int
	if ip.To4() != nil {
		curr = t.v4Root
		ip = ip.To4()
		bits = 32
	} else {
		curr = t.v6Root
		bits = 128
	}

	var bestValue string
	var found bool

	for i := 0; i < bits; i++ {
		if curr.value != "" {
			bestValue = curr.value
			found = true
		}

		byteIdx := i / 8
		bitIdx := 7 - (i % 8)
		bit := (ip[byteIdx] >> bitIdx) & 1

		if curr.children[bit] == nil {
			break
		}
		curr = curr.children[bit]
	}

	if curr != nil && curr.value != "" {
		bestValue = curr.value
		found = true
	}

	return bestValue, found
}

var defaultCDNRanges = map[string][]string{
	"cloudflare": {
		"173.245.48.0/20", "103.21.244.0/22", "103.22.200.0/22",
		"103.31.4.0/22", "141.101.64.0/18", "108.162.192.0/18",
		"190.93.240.0/20", "188.114.96.0/20", "197.234.240.0/22",
		"198.41.128.0/17", "162.158.0.0/15", "104.16.0.0/13",
		"104.24.0.0/14", "172.64.0.0/13", "131.0.72.0/22",
	},
	"cloudfront": {
		"13.32.0.0/15", "13.224.0.0/14", "13.35.0.0/16",
		"18.64.0.0/15", "18.66.0.0/16", "18.160.0.0/15",
		"18.172.0.0/15", "52.46.0.0/18", "52.84.0.0/15",
		"52.222.128.0/17", "54.182.0.0/16", "54.192.0.0/16",
		"54.230.0.0/16", "54.240.0.0/16", "99.84.0.0/16",
		"99.86.0.0/16", "108.156.0.0/14", "143.204.0.0/16",
		"204.246.176.0/20", "205.251.192.0/19",
	},
	"fastly": {
		"23.235.32.0/20", "43.249.72.0/22", "103.244.50.0/24",
		"103.245.222.0/23", "103.245.224.0/24", "104.156.80.0/20",
		"146.75.0.0/16", "151.101.0.0/16", "157.52.64.0/18",
		"167.99.192.0/18", "172.111.96.0/20", "185.31.16.0/22",
		"199.27.72.0/21", "199.232.0.0/16",
	},
	"akamai": {
		"23.0.0.0/12", "23.192.0.0/11", "184.24.0.0/13",
		"184.84.0.0/14", "184.50.0.0/15", "184.28.0.0/15",
		"72.246.0.0/15", "72.247.0.0/16", "80.239.128.0/17",
		"95.100.0.0/15", "96.16.0.0/12", "104.64.0.0/11",
		"118.214.0.0/16",
	},
	"gcore": {
		"88.212.240.0/20", "92.223.64.0/18", "95.85.64.0/18",
		"109.201.224.0/19", "188.93.96.0/20", "178.250.248.0/21",
	},
	"bunny": {
		"84.17.32.0/19", "185.243.218.0/24", "198.244.128.0/17",
		"212.102.32.0/19",
	},
	"cdn77": {
		"84.17.46.0/24", "89.238.128.0/18", "185.59.220.0/22",
	},
	"google": {
		"34.80.0.0/12", "34.96.0.0/12", "35.184.0.0/13",
		"35.192.0.0/11", "35.224.0.0/12", "35.240.0.0/13",
		"104.196.0.0/14",
	},
	"azure": {
		"13.64.0.0/11", "40.64.0.0/10", "52.145.0.0/16",
		"52.146.0.0/15", "52.148.0.0/14", "52.152.0.0/13",
		"52.160.0.0/11", "104.40.0.0/13",
	},
}

func getDisplayCDNName(filename string) string {
	name := strings.ToLower(filename)
	switch name {
	case "cloudflare":
		return "Cloudflare"
	case "cloudfront":
		return "AWS CloudFront"
	case "fastly":
		return "Fastly"
	case "akamai":
		return "Akamai"
	case "gcore":
		return "Gcore"
	case "bunny":
		return "Bunny CDN"
	case "cdn77":
		return "CDN77"
	case "google":
		return "Google Cloud CDN"
	case "azure":
		return "Microsoft Azure CDN"
	default:
		return strings.Title(name)
	}
}

func InitCDNRegistry(dataDir string) error {
	cdnDir := filepath.Join(dataDir, "cdn_ips")
	if err := os.MkdirAll(cdnDir, 0755); err != nil {
		return fmt.Errorf("failed to create cdn_ips directory: %w", err)
	}

	// 1. Seed files if empty or not exist
	for cdnName, ranges := range defaultCDNRanges {
		filePath := filepath.Join(cdnDir, cdnName+".txt")
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			content := strings.Join(ranges, "\n")
			_ = os.WriteFile(filePath, []byte(content), 0644)
		}
	}

	// 2. Read all .txt files and load into Radix tree
	files, err := os.ReadDir(cdnDir)
	if err != nil {
		return fmt.Errorf("failed to read cdn_ips directory: %w", err)
	}

	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(strings.ToLower(file.Name()), ".txt") {
			filePath := filepath.Join(cdnDir, file.Name())
			cdnName := strings.TrimSuffix(file.Name(), filepath.Ext(file.Name()))
			displayName := getDisplayCDNName(cdnName)

			f, err := os.Open(filePath)
			if err != nil {
				continue
			}

			scanner := bufio.NewScanner(f)
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				_ = GlobalCDNRegistry.Insert(line, displayName)
			}
			f.Close()
		}
	}

	return nil
}
