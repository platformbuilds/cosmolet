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

func TestLoadConfig_HappyPathParsesAllFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	body := `services:
  namespaces:
    - production
    - kube-system
loop_interval_seconds: 15
bgp:
  enabled: true
  asn: 64512
logging:
  level: debug
  format: json
frr:
  socket_path: /run/frr/vty
  config_path: /etc/frr/frr.conf
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error loading valid config: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("loaded config failed Validate: %v", err)
	}
	if len(cfg.Services.Namespaces) != 2 || cfg.Services.Namespaces[0] != "production" || cfg.Services.Namespaces[1] != "kube-system" {
		t.Fatalf("unexpected namespaces: %v", cfg.Services.Namespaces)
	}
	if cfg.LoopIntervalSeconds != 15 {
		t.Fatalf("expected loop interval 15, got %d", cfg.LoopIntervalSeconds)
	}
	if !cfg.BGP.Enabled {
		t.Fatal("expected BGP enabled")
	}
	if cfg.BGP.ASN != 64512 {
		t.Fatalf("expected ASN 64512, got %d", cfg.BGP.ASN)
	}
	if cfg.Logging.Level != "debug" || cfg.Logging.Format != "json" {
		t.Fatalf("unexpected logging config: %+v", cfg.Logging)
	}
	if cfg.FRR.SocketPath != "/run/frr/vty" {
		t.Fatalf("expected socket path /run/frr/vty, got %s", cfg.FRR.SocketPath)
	}
	if cfg.FRR.ConfigPath != "/etc/frr/frr.conf" {
		t.Fatalf("expected config path /etc/frr/frr.conf, got %s", cfg.FRR.ConfigPath)
	}
}

func TestLoadConfig_InvalidYAMLReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	body := "services: [unbalanced\n  namespaces: {{{\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	if _, err := LoadConfig(path); err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestLoadConfig_InvalidYAMLOffsetTypeReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	body := "loop_interval_seconds: notanumber\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	if _, err := LoadConfig(path); err == nil {
		t.Fatal("expected error for wrong-type YAML value")
	}
}

func TestLoadConfig_ValidateErrorPropagates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	// Valid YAML, but ASN out of range with BGP enabled: Validate must reject.
	body := "services:\n  namespaces:\n    - default\nbgp:\n  enabled: true\n  asn: 9999999999\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	if _, err := LoadConfig(path); err == nil {
		t.Fatal("expected Validate error to propagate for out-of-range ASN")
	}
}

func TestLoadConfig_ValidateErrorEmptyNamespace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	body := "services:\n  namespaces:\n    - \"\"\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	if _, err := LoadConfig(path); err == nil {
		t.Fatal("expected Validate error to propagate for empty namespace")
	}
}

func TestLoadConfig_OmittedFieldsKeepDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	// Omitted keys keep the defaults set by LoadConfig. BGP defaults to
	// enabled, so a valid minimal config must still supply an ASN.
	body := "services:\n  namespaces:\n    - only-namespace\nbgp:\n  asn: 65000\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error loading minimal config: %v", err)
	}
	if len(cfg.Services.Namespaces) != 1 || cfg.Services.Namespaces[0] != "only-namespace" {
		t.Fatalf("unexpected namespaces: %v", cfg.Services.Namespaces)
	}
	if cfg.LoopIntervalSeconds != 30 {
		t.Fatalf("expected default loop interval 30, got %d", cfg.LoopIntervalSeconds)
	}
	if !cfg.BGP.Enabled {
		t.Fatal("expected BGP to be enabled by default")
	}
	if cfg.BGP.ASN != 65000 {
		t.Fatalf("expected ASN 65000 from file, got %d", cfg.BGP.ASN)
	}
	if cfg.Logging.Level != "info" || cfg.Logging.Format != "text" {
		t.Fatalf("unexpected default logging: %+v", cfg.Logging)
	}
	if cfg.FRR.SocketPath != "/var/run/frr" {
		t.Fatalf("expected default socket path /var/run/frr, got %s", cfg.FRR.SocketPath)
	}
}

func TestLoadConfig_MissingASNWithDefaultBGPEnabledFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	// BGP defaults to enabled with ASN 0, which is invalid: fail-closed.
	body := "services:\n  namespaces:\n    - only-namespace\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	if _, err := LoadConfig(path); err == nil {
		t.Fatal("expected error: default BGP enabled with zero ASN is invalid")
	}
}

func TestGetNamespaces(t *testing.T) {
	c := &Config{Services: ServicesConfig{Namespaces: []string{"a", "b"}}}
	got := c.GetNamespaces()
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("GetNamespaces() = %v", got)
	}
}

func TestGetLoopInterval(t *testing.T) {
	c := &Config{LoopIntervalSeconds: 42}
	if got := c.GetLoopInterval(); got != 42 {
		t.Fatalf("GetLoopInterval() = %d", got)
	}
}

func TestGetBGPASN(t *testing.T) {
	c := &Config{BGP: BGPConfig{ASN: 65021}}
	if got := c.GetBGPASN(); got != 65021 {
		t.Fatalf("GetBGPASN() = %d", got)
	}
}

func TestIsBGPEnabled(t *testing.T) {
	if (&Config{}).IsBGPEnabled() {
		t.Fatal("expected BGP disabled for zero-value config")
	}
	c := &Config{BGP: BGPConfig{Enabled: true}}
	if !c.IsBGPEnabled() {
		t.Fatal("expected BGP enabled")
	}
}

func TestGetFRRSocketPath(t *testing.T) {
	c := &Config{FRR: FRRConfig{SocketPath: "/run/frr/vty"}}
	if got := c.GetFRRSocketPath(); got != "/run/frr/vty" {
		t.Fatalf("GetFRRSocketPath() = %s", got)
	}
}

func TestGetFRRConfigPath(t *testing.T) {
	c := &Config{FRR: FRRConfig{ConfigPath: "/etc/frr/frr.conf"}}
	if got := c.GetFRRConfigPath(); got != "/etc/frr/frr.conf" {
		t.Fatalf("GetFRRConfigPath() = %s", got)
	}
}

func TestGetVtyshSocketArgs_Set(t *testing.T) {
	c := &Config{FRR: FRRConfig{SocketPath: "/run/frr/vty"}}
	got := c.GetVtyshSocketArgs()
	if len(got) != 2 || got[0] != "-s" || got[1] != "/run/frr/vty" {
		t.Fatalf("GetVtyshSocketArgs() = %v, want [-s /run/frr/vty]", got)
	}
}

func TestGetVtyshSocketArgs_Empty(t *testing.T) {
	c := &Config{}
	if got := c.GetVtyshSocketArgs(); got != nil {
		t.Fatalf("GetVtyshSocketArgs() = %v, want nil when socket path empty", got)
	}
}

func TestYAMLSnakeCaseTagsMapToCorrectFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	body := "services:\n  namespaces:\n    - ns-a\nloop_interval_seconds: 7\nbgp:\n  enabled: true\n  asn: 100\nlogging:\n  level: warn\n  format: json\nfrr:\n  socket_path: /tmp/frr.sock\n  config_path: /tmp/frr.conf\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.LoopIntervalSeconds != 7 {
		t.Errorf("loop_interval_seconds did not map to LoopIntervalSeconds: got %d", cfg.LoopIntervalSeconds)
	}
	if cfg.BGP.ASN != 100 {
		t.Errorf("bgp.asn did not map to BGP.ASN: got %d", cfg.BGP.ASN)
	}
	if cfg.Logging.Level != "warn" {
		t.Errorf("logging.level did not map to Logging.Level: got %q", cfg.Logging.Level)
	}
	if cfg.FRR.SocketPath != "/tmp/frr.sock" {
		t.Errorf("frr.socket_path did not map to FRR.SocketPath: got %q", cfg.FRR.SocketPath)
	}
	if cfg.FRR.ConfigPath != "/tmp/frr.conf" {
		t.Errorf("frr.config_path did not map to FRR.ConfigPath: got %q", cfg.FRR.ConfigPath)
	}
	if len(cfg.Services.Namespaces) != 1 || cfg.Services.Namespaces[0] != "ns-a" {
		t.Errorf("services.namespaces did not map to Services.Namespaces: got %v", cfg.Services.Namespaces)
	}
}
