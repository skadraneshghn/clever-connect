package dns

import (
	"syscall"

	"clever-connect/internal/logger"
)

// SetHighOpenFileLimits ephemerally raises system limits for file descriptors
func SetHighOpenFileLimits() {
	var rLimit syscall.Rlimit
	err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rLimit)
	if err != nil {
		logger.Error("DNS", "Failed to retrieve open files limit", "error", err)
		return
	}
	
	// Raise limit to max permitted by kernel
	rLimit.Cur = rLimit.Max
	err = syscall.Setrlimit(syscall.RLIMIT_NOFILE, &rLimit)
	if err != nil {
		logger.Error("DNS", "Failed to increase open files limit", "error", err)
	} else {
		logger.Info("DNS", "System file descriptor limit successfully raised to maximum", "limit", rLimit.Max)
	}
}
