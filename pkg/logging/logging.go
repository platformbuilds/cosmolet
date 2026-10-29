// pkg/logging/logging.go
package logging

import (
	"io"
	"log"
	"os"
	"strings"
	"sync"
)

// levelRank orders log levels from most to least verbose. A level is
// emitted when its rank is <= the configured threshold's rank.
var levelRank = map[string]int{
	"debug": 0,
	"info":  1,
	"warn":  2,
	"error": 3,
}

type filter struct {
	mu      sync.Mutex
	level   string
	rank    int
	logger  *log.Logger
}

// newFilter wraps stdlib log with a minimal level gate. The filter itself is
// package-internal; callers use the package-level functions below.
var (
	once   sync.Once
	std    = log.New(os.Stderr, "", log.LstdFlags)
	filt   = &filter{level: "info", rank: levelRank["info"], logger: std}
	levels = map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
)

// IsKnownLevel reports whether level is one of debug, info, warn, error.
func IsKnownLevel(level string) bool { return levels[level] }

// SetLevel configures the minimum level that will be emitted. Unknown levels
// fall back to "info". This is the single place the documented -log-level
// flag and cfg.Logging.Level are applied.
func SetLevel(level string) {
	filt.mu.Lock()
	defer filt.mu.Unlock()
	if !levels[level] {
		level = "info"
	}
	filt.level = level
	filt.rank = levelRank[level]
}

// enabled reports whether a message at the given level should be emitted.
func enabled(level string) bool {
	filt.mu.Lock()
	r := filt.rank
	filt.mu.Unlock()
	if rank, ok := levelRank[level]; ok {
		return rank >= r
	}
	return true
}

// Debug logs at debug level.
func Debug(format string, args ...interface{}) { logIf("debug", format, args...) }
// Info logs at info level.
func Info(format string, args ...interface{})  { logIf("info", format, args...) }
// Warn logs at warn level.
func Warn(format string, args ...interface{})  { logIf("warn", format, args...) }
// Error logs at error level.
func Error(format string, args ...interface{}) { logIf("error", format, args...) }

// logIf emits the message through the shared logger iff the level gate allows.
func logIf(level, format string, args ...interface{}) {
	if !enabled(level) {
		return
	}
	filt.mu.Lock()
	filt.logger.Printf(format, args...)
	filt.mu.Unlock()
}

// SetOutput overrides the underlying writer (used by tests to capture output).
func SetOutput(w io.Writer) {
	filt.mu.Lock()
	filt.logger.SetOutput(w)
	filt.mu.Unlock()
}

// Discard sends output to io.Discard (kept for API symmetry / future use).
func Discard() { SetOutput(io.Discard) }

// CurrentLevel returns the active minimum level.
func CurrentLevel() string {
	filt.mu.Lock()
	defer filt.mu.Unlock()
	return strings.ToLower(filt.level)
}
