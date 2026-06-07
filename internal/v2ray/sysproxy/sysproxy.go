package sysproxy

import (
	"runtime"
)

// SetSystemProxy enables system proxy settings across Windows, macOS, and Linux
func SetSystemProxy(socksPort, httpPort int) error {
	switch runtime.GOOS {
	case "linux":
		return setLinuxProxy(socksPort, httpPort)
	case "darwin":
		return setMacProxy(socksPort, httpPort)
	case "windows":
		return setWindowsProxy(socksPort, httpPort)
	}
	return nil
}

// ClearSystemProxy disables system proxy settings
func ClearSystemProxy() error {
	switch runtime.GOOS {
	case "linux":
		return clearLinuxProxy()
	case "darwin":
		return clearMacProxy()
	case "windows":
		return clearWindowsProxy()
	}
	return nil
}
