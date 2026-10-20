package logging

import (
	"fmt"
	"log"
	"strings"
	"sync/atomic"
)

var debug atomic.Bool

// Configure enables optional diagnostics while preserving the standard logger output.
func Configure(level string) error {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "", "info":
		debug.Store(false)
	case "debug":
		debug.Store(true)
	default:
		return fmt.Errorf("invalid FSM_LOG_LEVEL: expected info or debug")
	}
	return nil
}

func DebugEnabled() bool { return debug.Load() }

func Debugf(format string, args ...interface{}) {
	if debug.Load() {
		log.Printf("DEBUG "+format, args...)
	}
}
