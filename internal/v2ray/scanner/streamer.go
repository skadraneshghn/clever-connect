package scanner

import (
	"bufio"
	"context"
	"io"
	"net"
	"os"
	"strings"
)

// StreamAddresses parses address sources from a file or raw string, expands CIDRs, and streams to a channel.
func StreamAddresses(ctx context.Context, source string, isFile bool) (<-chan string, error) {
	var reader io.Reader
	if isFile {
		f, err := os.Open(source)
		if err != nil {
			return nil, err
		}
		// Goroutine will close the file
		reader = f
	} else {
		reader = strings.NewReader(source)
	}

	outChan := make(chan string, 1000)

	go func() {
		defer func() {
			if isFile {
				if fclose, ok := reader.(io.Closer); ok {
					_ = fclose.Close()
				}
			}
			close(outChan)
		}()

		scanner := bufio.NewScanner(reader)
		for scanner.Scan() {
			select {
			case <-ctx.Done():
				return
			default:
			}

			line := scanner.Text()
			// Remove comments
			if idx := strings.Index(line, "#"); idx >= 0 {
				line = line[:idx]
			}
			if idx := strings.Index(line, "//"); idx >= 0 {
				line = line[:idx]
			}
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}

			// Handle comma-separated values (CSV)
			parts := strings.Split(line, ",")
			for _, part := range parts {
				part = strings.TrimSpace(part)
				if part == "" {
					continue
				}

				if strings.Contains(part, "/") {
					// CIDR block
					ips, err := UnpackCIDR(part)
					if err == nil {
						for _, ipStr := range ips {
							select {
							case outChan <- ipStr:
							case <-ctx.Done():
								return
							}
						}
					}
				} else {
					// Single IP address
					parsedIP := net.ParseIP(part)
					if parsedIP != nil {
						select {
						case outChan <- parsedIP.String():
						case <-ctx.Done():
							return
						}
					}
				}
			}
		}
	}()

	return outChan, nil
}

// UnpackCIDR parses a CIDR block and returns all sequential individual host IPs
func UnpackCIDR(cidr string) ([]string, error) {
	ip, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, err
	}

	var ips []string
	for nextIP := ip.Mask(ipnet.Mask); ipnet.Contains(nextIP); incrementIP(nextIP) {
		// Copy the IP to avoid mutation side effects
		temp := make(net.IP, len(nextIP))
		copy(temp, nextIP)
		ips = append(ips, temp.String())
	}

	// Filter network and broadcast addresses for IPv4 blocks larger than /31
	if strings.Contains(cidr, ".") && len(ips) > 2 {
		return ips[1 : len(ips)-1], nil
	}
	return ips, nil
}

func incrementIP(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}
