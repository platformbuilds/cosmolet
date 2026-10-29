package config

import (
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
