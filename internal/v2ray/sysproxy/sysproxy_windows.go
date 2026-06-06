//go:build windows

package sysproxy

import (
	"fmt"
	"os/exec"
	"strconv"
)

func setWindowsProxy(socksPort, httpPort int) error {
	serverStr := fmt.Sprintf("http=127.0.0.1:%d;https=127.0.0.1:%d;socks=127.0.0.1:%d", httpPort, httpPort, socksPort)
	_ = exec.Command("reg", "add", `HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`, "/v", "ProxyEnable", "/t", "REG_DWORD", "/d", "1", "/f").Run()
	_ = exec.Command("reg", "add", `HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`, "/v", "ProxyServer", "/t", "REG_SZ", "/d", serverStr, "/f").Run()
	_ = exec.Command("reg", "add", `HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`, "/v", "ProxyOverride", "/t", "REG_SZ", "/d", "<local>", "/f").Run()

	// Notify settings change using a small PowerShell script
	_ = exec.Command("powershell", "-WindowStyle", "Hidden", "-Command", `
		$signature = @'
		[DllImport("wininet.dll", SetLastError = true)]
		public static extern bool InternetSetOption(IntPtr hInternet, int dwOption, IntPtr lpBuffer, int dwBufferLength);
		'@
		$type = Add-Type -MemberDefinition $signature -Name "WinInet" -Namespace "Win32" -PassThru
		$type::InternetSetOption([IntPtr]::Zero, 39, [IntPtr]::Zero, 0)
		$type::InternetSetOption([IntPtr]::Zero, 37, [IntPtr]::Zero, 0)
	`).Run()

	return nil
}

func clearWindowsProxy() error {
	_ = exec.Command("reg", "add", `HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`, "/v", "ProxyEnable", "/t", "REG_DWORD", "/d", "0", "/f").Run()

	// Notify settings change using a small PowerShell script
	_ = exec.Command("powershell", "-WindowStyle", "Hidden", "-Command", `
		$signature = @'
		[DllImport("wininet.dll", SetLastError = true)]
		public static extern bool InternetSetOption(IntPtr hInternet, int dwOption, IntPtr lpBuffer, int dwBufferLength);
		'@
		$type = Add-Type -MemberDefinition $signature -Name "WinInet" -Namespace "Win32" -PassThru
		$type::InternetSetOption([IntPtr]::Zero, 39, [IntPtr]::Zero, 0)
		$type::InternetSetOption([IntPtr]::Zero, 37, [IntPtr]::Zero, 0)
	`).Run()

	return nil
}

// Fallback stubs for other OS platforms
func setLinuxProxy(socksPort, httpPort int) error { return nil }
func clearLinuxProxy() error                 { return nil }
func setMacProxy(socksPort, httpPort int) error { return nil }
func clearMacProxy() error                 { return nil }
