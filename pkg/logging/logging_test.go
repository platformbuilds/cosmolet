package logging

import (
	"bytes"
	"strings"
	"testing"
)

func TestSetLevelFiltersDebugWhenInfo(t *testing.T) {
	var buf bytes.Buffer
	SetOutput(&buf)
	SetLevel("info")

	Debug("should be hidden")
	Info("should show")
	got := buf.String()
	if strings.Contains(got, "should be hidden") {
		t.Fatalf("debug message leaked at info level: %q", got)
	}
	if !strings.Contains(got, "should show") {
		t.Fatalf("info message missing at info level: %q", got)
	}
}

func TestSetLevelWarnHidesInfoAndDebug(t *testing.T) {
	var buf bytes.Buffer
	SetOutput(&buf)
	SetLevel("warn")

	Debug("d")
	Info("i")
	Warn("w")
	Error("e")
	got := buf.String()
	if strings.Contains(got, "d") || strings.Contains(got, "i") {
		t.Fatalf("debug/info leaked at warn level: %q", got)
	}
	if !strings.Contains(got, "w") || !strings.Contains(got, "e") {
		t.Fatalf("warn/error missing at warn level: %q", got)
	}
}

func TestSetLevelDebugShowsAll(t *testing.T) {
	var buf bytes.Buffer
	SetOutput(&buf)
	SetLevel("debug")

	Debug("d")
	Info("i")
	Warn("w")
	Error("e")
	got := buf.String()
	for _, want := range []string{"d", "i", "w", "e"} {
		if !strings.Contains(got, want) {
			t.Fatalf("level debug should show all; missing %q in %q", want, got)
		}
	}
}

func TestUnknownLevelFallsBackToInfo(t *testing.T) {
	SetLevel("bogus")
	if got := CurrentLevel(); got != "info" {
		t.Fatalf("expected fallback to info, got %q", got)
	}
}

func TestErrorAlwaysEmitted(t *testing.T) {
	var buf bytes.Buffer
	SetOutput(&buf)
	SetLevel("error")

	Info("hidden")
	Error("shown")
	got := buf.String()
	if strings.Contains(got, "hidden") {
		t.Fatalf("info leaked at error level: %q", got)
	}
	if !strings.Contains(got, "shown") {
		t.Fatalf("error message missing: %q", got)
	}
}

func TestIsKnownLevel(t *testing.T) {
	for _, ok := range []string{"debug", "info", "warn", "error"} {
		if !IsKnownLevel(ok) {
			t.Fatalf("expected %q to be a known level", ok)
		}
	}
	if IsKnownLevel("trace") {
		t.Fatal("expected trace to be unknown")
	}
}
