//go:build linux

package sysproxy

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
)

func setLinuxProxy(socksPort, httpPort int) error {
	// GNOME (gsettings)
	_ = exec.Command("gsettings", "set", "org.gnome.system.proxy", "mode", "manual").Run()
	_ = exec.Command("gsettings", "set", "org.gnome.system.proxy.socks", "host", "127.0.0.1").Run()
	_ = exec.Command("gsettings", "set", "org.gnome.system.proxy.socks", "port", strconv.Itoa(socksPort)).Run()
	_ = exec.Command("gsettings", "set", "org.gnome.system.proxy.http", "host", "127.0.0.1").Run()
	_ = exec.Command("gsettings", "set", "org.gnome.system.proxy.http", "port", strconv.Itoa(httpPort)).Run()
	_ = exec.Command("gsettings", "set", "org.gnome.system.proxy.https", "host", "127.0.0.1").Run()
	_ = exec.Command("gsettings", "set", "org.gnome.system.proxy.https", "port", strconv.Itoa(httpPort)).Run()

	// KDE (kwriteconfig5 + dbus)
	_ = exec.Command("kwriteconfig5", "--file", "kioslaverc", "--group", "Proxy Settings", "--key", "ProxyType", "1").Run()
	_ = exec.Command("kwriteconfig5", "--file", "kioslaverc", "--group", "Proxy Settings", "--key", "socksProxy", fmt.Sprintf("socks://127.0.0.1 %d", socksPort)).Run()
	_ = exec.Command("kwriteconfig5", "--file", "kioslaverc", "--group", "Proxy Settings", "--key", "httpProxy", fmt.Sprintf("http://127.0.0.1 %d", httpPort)).Run()
	_ = exec.Command("kwriteconfig5", "--file", "kioslaverc", "--group", "Proxy Settings", "--key", "httpsProxy", fmt.Sprintf("http://127.0.0.1 %d", httpPort)).Run()
	_ = exec.Command("dbus-send", "--type=signal", "/KIO/Scheduler", "org.kde.KIO.Scheduler.reconfigure", "string:").Run()

	// Write to env profile file
	homeDir, err := os.UserHomeDir()
	if err == nil {
		envPath := filepath.Join(homeDir, ".config", "cleverconnect", "proxy.env")
		_ = os.MkdirAll(filepath.Dir(envPath), 0755)
		content := fmt.Sprintf("export http_proxy=http://127.0.0.1:%d\nexport https_proxy=http://127.0.0.1:%d\nexport socks_proxy=socks5://127.0.0.1:%d\nexport ALL_PROXY=socks5://127.0.0.1:%d\n", httpPort, httpPort, socksPort, socksPort)
		_ = os.WriteFile(envPath, []byte(content), 0644)
	}

	return nil
}

func clearLinuxProxy() error {
	// GNOME (gsettings)
	_ = exec.Command("gsettings", "set", "org.gnome.system.proxy", "mode", "none").Run()

	// KDE (kwriteconfig5 + dbus)
	_ = exec.Command("kwriteconfig5", "--file", "kioslaverc", "--group", "Proxy Settings", "--key", "ProxyType", "0").Run()
	_ = exec.Command("dbus-send", "--type=signal", "/KIO/Scheduler", "org.kde.KIO.Scheduler.reconfigure", "string:").Run()

	// Clear env profile file
	homeDir, err := os.UserHomeDir()
	if err == nil {
		envPath := filepath.Join(homeDir, ".config", "cleverconnect", "proxy.env")
		_ = os.Remove(envPath)
	}

	return nil
}

// Fallback stubs for other OS platforms so compile doesn't fail
func setMacProxy(socksPort, httpPort int) error { return nil }
func clearMacProxy() error                 { return nil }
func setWindowsProxy(socksPort, httpPort int) error { return nil }
func clearWindowsProxy() error                 { return nil }
