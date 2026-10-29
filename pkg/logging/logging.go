// pkg/logging/logging.go
package logging

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"time"
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
	format  string
	logger  *log.Logger
	jsonOut io.Writer
}

// jsonLogEntry is the shape emitted when format=json (Finding 34).
type jsonLogEntry struct {
	Timestamp string `json:"ts"`
	Level     string `json:"level"`
	Msg       string `json:"msg"`
}

// newFilter wraps stdlib log with a minimal level gate. The filter itself is
// package-internal; callers use the package-level functions below.
var (
	once   sync.Once
	std    = log.New(os.Stderr, "", log.LstdFlags)
	filt   = &filter{level: "info", rank: levelRank["info"], format: "text", logger: std, jsonOut: os.Stderr}
	levels = map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	formats = map[string]bool{"text": true, "json": true}
)

// IsKnownLevel reports whether level is one of debug, info, warn, error.
func IsKnownLevel(level string) bool { return levels[level] }

// IsKnownFormat reports whether format is one of text, json.
func IsKnownFormat(format string) bool { return formats[format] }

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

// SetFormat configures the output format (text or json). Unknown formats fall
// back to "text". This is the single place the documented -log-format flag and
// cfg.Logging.Format are applied (Finding 34). When set to "json", each log
// line is emitted as a JSON object with ts/level/msg fields.
func SetFormat(format string) {
	filt.mu.Lock()
	defer filt.mu.Unlock()
	if !formats[format] {
		format = "text"
	}
	filt.format = format
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
// It dispatches on the configured format (text or json) (Finding 34).
func logIf(level, format string, args ...interface{}) {
	if !enabled(level) {
		return
	}
	msg := fmt.Sprintf(format, args...)
	filt.mu.Lock()
	if filt.format == "json" {
		entry := jsonLogEntry{
			Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
			Level:     level,
			Msg:       msg,
		}
		if b, err := json.Marshal(entry); err == nil {
			b = append(b, '\n')
			filt.jsonOut.Write(b)
		} else {
			filt.logger.Printf("%s %s", level, msg)
		}
	} else {
		filt.logger.Printf("%s", msg)
	}
	filt.mu.Unlock()
}

// SetOutput overrides the underlying text writer (used by tests to capture
// output).
func SetOutput(w io.Writer) {
	filt.mu.Lock()
	filt.logger.SetOutput(w)
	filt.mu.Unlock()
}

// SetJSONOutput overrides the writer used when format=json (used by tests to
// capture JSON output).
func SetJSONOutput(w io.Writer) {
	filt.mu.Lock()
	filt.jsonOut = w
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

// CurrentFormat returns the active output format (text or json).
func CurrentFormat() string {
	filt.mu.Lock()
	defer filt.mu.Unlock()
	return strings.ToLower(filt.format)
}
