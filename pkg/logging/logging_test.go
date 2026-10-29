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

// Finding 34: SetFormat("json") must emit JSON lines, not plain text.
func TestSetFormatJSONEmitsJSONLines(t *testing.T) {
	var jsonBuf bytes.Buffer
	SetLevel("info")
	SetFormat("json")
	SetJSONOutput(&jsonBuf)
	defer SetFormat("text") // restore for other tests

	Info("hello %d", 42)
	got := jsonBuf.String()
	if !strings.Contains(got, `"level":"info"`) {
		t.Fatalf("expected a JSON line with level=info, got %q", got)
	}
	if !strings.Contains(got, `"msg":"hello 42"`) {
		t.Fatalf("expected msg field to be formatted, got %q", got)
	}
	if !strings.Contains(got, `"ts":"`) {
		t.Fatalf("expected a ts (timestamp) field, got %q", got)
	}
	if !strings.HasPrefix(strings.TrimSpace(got), "{") {
		t.Fatalf("expected the line to start with '{' (JSON object), got %q", got)
	}
}

func TestSetFormatTextStaysPlainText(t *testing.T) {
	var buf bytes.Buffer
	SetLevel("info")
	SetFormat("text")
	SetOutput(&buf)

	Info("plain message")
	got := buf.String()
	if !strings.Contains(got, "plain message") {
		t.Fatalf("expected plain text output, got %q", got)
	}
	if strings.Contains(got, `"msg"`) {
		t.Fatalf("text format must not emit JSON, got %q", got)
	}
}

func TestUnknownFormatFallsBackToText(t *testing.T) {
	SetFormat("bogus")
	if got := CurrentFormat(); got != "text" {
		t.Fatalf("expected fallback to text, got %q", got)
	}
}

func TestIsKnownFormat(t *testing.T) {
	if !IsKnownFormat("text") || !IsKnownFormat("json") {
		t.Fatal("expected text and json to be known formats")
	}
	if IsKnownFormat("xml") {
		t.Fatal("expected xml to be unknown")
	}
}
