//go:build darwin

package sysproxy

import (
	"os/exec"
	"strconv"
)

var commonInterfaces = []string{"Wi-Fi", "Ethernet", "Thunderbolt Bridge"}

func setMacProxy(socksPort, httpPort int) error {
	for _, iface := range commonInterfaces {
		_ = exec.Command("networksetup", "-setsocksfirewallproxy", iface, "127.0.0.1", strconv.Itoa(socksPort)).Run()
		_ = exec.Command("networksetup", "-setsocksfirewallproxystate", iface, "on").Run()

		_ = exec.Command("networksetup", "-setwebproxy", iface, "127.0.0.1", strconv.Itoa(httpPort)).Run()
		_ = exec.Command("networksetup", "-setwebproxystate", iface, "on").Run()

		_ = exec.Command("networksetup", "-setsecurewebproxy", iface, "127.0.0.1", strconv.Itoa(httpPort)).Run()
		_ = exec.Command("networksetup", "-setsecurewebproxystate", iface, "on").Run()
	}
	return nil
}

func clearMacProxy() error {
	for _, iface := range commonInterfaces {
		_ = exec.Command("networksetup", "-setsocksfirewallproxystate", iface, "off").Run()
		_ = exec.Command("networksetup", "-setwebproxystate", iface, "off").Run()
		_ = exec.Command("networksetup", "-setsecurewebproxystate", iface, "off").Run()
	}
	return nil
}

// Fallback stubs for other OS platforms
func setLinuxProxy(socksPort, httpPort int) error { return nil }
func clearLinuxProxy() error                 { return nil }
func setWindowsProxy(socksPort, httpPort int) error { return nil }
func clearWindowsProxy() error                 { return nil }
