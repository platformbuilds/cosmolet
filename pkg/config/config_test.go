package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func validConfig() *Config {
	return &Config{
		Services: ServicesConfig{Namespaces: []string{"default"}},
		LoopIntervalSeconds: 30,
		BGP:        BGPConfig{Enabled: true, ASN: 65000},
		Logging:    LoggingConfig{Level: "info", Format: "text"},
		FRR:        FRRConfig{SocketPath: "/var/run/frr"},
	}
}

func TestValidate_ASNZeroIsInvalid(t *testing.T) {
	c := validConfig()
	c.BGP.ASN = 0
	if err := c.Validate(); err == nil {
		t.Fatal("expected error for ASN 0 with bgp enabled")
	}
}

func TestValidate_ASNNegativeIsInvalid(t *testing.T) {
	c := validConfig()
	c.BGP.ASN = -1
	if err := c.Validate(); err == nil {
		t.Fatal("expected error for negative ASN")
	}
}

func TestValidate_ASNTooLargeIsInvalid(t *testing.T) {
	c := validConfig()
	c.BGP.ASN = 4294967296
	if err := c.Validate(); err == nil {
		t.Fatal("expected error for ASN > 2^32-1")
	}
}

func TestValidate_ASNMaxIsValid(t *testing.T) {
	c := validConfig()
	c.BGP.ASN = 4294967295
	if err := c.Validate(); err != nil {
		t.Fatalf("unexpected error for max ASN: %v", err)
	}
}

func TestValidate_ASNZeroAllowedWhenBGPDIsabled(t *testing.T) {
	c := validConfig()
	c.BGP.Enabled = false
	c.BGP.ASN = 0
	if err := c.Validate(); err != nil {
		t.Fatalf("ASN 0 should be allowed when BGP is disabled: %v", err)
	}
}

func TestValidate_DefaultsAreValid(t *testing.T) {
	c := validConfig()
	if err := c.Validate(); err != nil {
		t.Fatalf("expected defaults to be valid: %v", err)
	}
}

func TestValidate_NoNamespacesIsInvalid(t *testing.T) {
	c := validConfig()
	c.Services.Namespaces = nil
	if err := c.Validate(); err == nil {
		t.Fatal("expected error for empty namespaces")
	}
}

func TestValidate_BadLogLevelIsInvalid(t *testing.T) {
	c := validConfig()
	c.Logging.Level = "verbose"
	if err := c.Validate(); err == nil {
		t.Fatal("expected error for bad log level")
	}
}

func TestValidate_ZeroLoopIntervalIsInvalid(t *testing.T) {
	c := validConfig()
	c.LoopIntervalSeconds = 0
	if err := c.Validate(); err == nil {
		t.Fatal("expected error for zero loop interval")
	}
}

func TestValidate_EmptySocketPathIsInvalid(t *testing.T) {
	c := validConfig()
	c.FRR.SocketPath = ""
	if err := c.Validate(); err == nil {
		t.Fatal("expected error for empty socket path")
	}
}

func TestValidate_EmptyStringNamespaceIsInvalid(t *testing.T) {
	c := validConfig()
	c.Services.Namespaces = []string{"default", ""}
	if err := c.Validate(); err == nil {
		t.Fatal("expected error for empty-string namespace (all-namespaces wildcard)")
	}
}

func TestValidate_WhitespaceOnlyNamespaceIsInvalid(t *testing.T) {
	c := validConfig()
	c.Services.Namespaces = []string{"   "}
	if err := c.Validate(); err == nil {
		t.Fatal("expected error for whitespace-only namespace")
	}
}

func TestValidate_NonEmptyNamespacesAreValid(t *testing.T) {
	c := validConfig()
	c.Services.Namespaces = []string{"default", "kube-system", "prod"}
	if err := c.Validate(); err != nil {
		t.Fatalf("unexpected error for normal namespaces: %v", err)
	}
}

func TestValidate_LoopIntervalAboveMaxIsInvalid(t *testing.T) {
	c := validConfig()
	c.LoopIntervalSeconds = 3601
	if err := c.Validate(); err == nil {
		t.Fatal("expected error for loop_interval_seconds above the max")
	}
}

func TestValidate_EnormousLoopIntervalIsInvalid(t *testing.T) {
	c := validConfig()
	c.LoopIntervalSeconds = 999999999
	if err := c.Validate(); err == nil {
		t.Fatal("expected error for an enormous loop_interval_seconds")
	}
}

func TestValidate_LoopIntervalAtMaxIsValid(t *testing.T) {
	c := validConfig()
	c.LoopIntervalSeconds = 3600
	if err := c.Validate(); err != nil {
		t.Fatalf("loop_interval_seconds at the max should be valid: %v", err)
	}
}

func TestLoadConfig_MissingFileFailsClosed(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "does-not-exist.yaml")
	if _, err := LoadConfig(missing); err == nil {
		t.Fatal("expected error when the config file is missing (fail-closed)")
	} else if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected a 'not found' error, got: %v", err)
	}
}

func TestLoadConfig_ValidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	body := "services:\n  namespaces:\n    - default\nloop_interval_seconds: 30\nbgp:\n  enabled: true\n  asn: 65000\nlogging:\n  level: info\n  format: text\nfrr:\n  socket_path: /var/run/frr\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error loading valid config: %v", err)
	}
	if cfg.BGP.ASN != 65000 {
		t.Fatalf("expected ASN 65000, got %d", cfg.BGP.ASN)
	}
}
