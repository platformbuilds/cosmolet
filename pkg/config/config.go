// pkg/config/config.go
package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v2"
)

// Config represents the complete configuration structure
type Config struct {
	Services            ServicesConfig `yaml:"services"`
	LoopIntervalSeconds int            `yaml:"loop_interval_seconds"`
	BGP                 BGPConfig      `yaml:"bgp,omitempty"`
	Logging             LoggingConfig  `yaml:"logging,omitempty"`
	FRR                 FRRConfig      `yaml:"frr,omitempty"`
}

// ServicesConfig contains service discovery configuration
type ServicesConfig struct {
	Namespaces []string `yaml:"namespaces"`
}

// BGPConfig contains BGP-specific configuration
type BGPConfig struct {
	Enabled bool `yaml:"enabled"`
	ASN     int  `yaml:"asn,omitempty"`
}

// LoggingConfig contains logging configuration
type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

// FRRConfig contains FRR-specific configuration
type FRRConfig struct {
	SocketPath string `yaml:"socket_path"`
}

// LoadConfig loads configuration from the specified file path
func LoadConfig(configPath string) (*Config, error) {
	// Set defaults
	config := &Config{
		Services: ServicesConfig{
			Namespaces: []string{"default"},
		},
		LoopIntervalSeconds: 30,
		BGP: BGPConfig{
			Enabled: true,
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "text",
		},
		FRR: FRRConfig{
			SocketPath: "/var/run/frr",
		},
	}

	// Check if config file exists. Fail closed: a BGP routing daemon must not
	// silently run on fail-open defaults (BGP enabled, ASN 0) when its config
	// is missing or mispathed (Finding 24).
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("config file %s not found (fail-closed; create it or point -config at a valid file)", configPath)
	}

	// Read config file
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %v", configPath, err)
	}

	// Parse YAML
	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("failed to parse config file %s: %v", configPath, err)
	}

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %v", err)
	}

	return config, nil
}

// maxLoopIntervalSeconds caps loop_interval_seconds. An unbounded value lets a
// misconfigured daemon sleep for years between loops, wedging the control loop
// and tripping the liveness staleness check permanently (Finding 23).
const maxLoopIntervalSeconds = 3600

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	// Validate services configuration. An empty entry ("") is the Kubernetes
	// all-namespaces wildcard, which would silently escalate a scoped
	// allow-list to cluster-wide discovery, so it is rejected (Finding 22).
	if len(c.Services.Namespaces) == 0 {
		return fmt.Errorf("at least one namespace must be specified")
	}
	for _, ns := range c.Services.Namespaces {
		if strings.TrimSpace(ns) == "" {
			return fmt.Errorf("services.namespaces must not contain empty entries (the empty string means all namespaces)")
		}
	}

	// Validate loop interval
	if c.LoopIntervalSeconds <= 0 {
		return fmt.Errorf("loop_interval_seconds must be positive")
	}
	if c.LoopIntervalSeconds > maxLoopIntervalSeconds {
		return fmt.Errorf("loop_interval_seconds must be at most %d (got %d)", maxLoopIntervalSeconds, c.LoopIntervalSeconds)
	}

	// Validate logging level
	validLogLevels := map[string]bool{
		"debug": true,
		"info":  true,
		"warn":  true,
		"error": true,
	}
	if !validLogLevels[c.Logging.Level] {
		return fmt.Errorf("invalid log level: %s (must be debug, info, warn, or error)", c.Logging.Level)
	}

	// Validate logging format
	validLogFormats := map[string]bool{
		"text": true,
		"json": true,
	}
	if !validLogFormats[c.Logging.Format] {
		return fmt.Errorf("invalid log format: %s (must be text or json)", c.Logging.Format)
	}

	// Validate FRR socket path
	if c.FRR.SocketPath == "" {
		return fmt.Errorf("frr.socket_path cannot be empty")
	}

	// Validate BGP ASN when BGP is enabled. FRR requires a non-zero ASN;
	// "router bgp 0" is rejected, so 0 (the zero value when unset) is invalid.
	if c.BGP.Enabled {
		if c.BGP.ASN < 1 || c.BGP.ASN > 4294967295 {
			return fmt.Errorf("bgp.asn must be between 1 and 4294967295 when bgp.enabled is true (got %d)", c.BGP.ASN)
		}
	}

	return nil
}

// GetNamespaces returns the list of namespaces to monitor
func (c *Config) GetNamespaces() []string {
	return c.Services.Namespaces
}

// GetLoopInterval returns the loop interval duration
func (c *Config) GetLoopInterval() int {
	return c.LoopIntervalSeconds
}

// IsBGPEnabled returns whether BGP is enabled
func (c *Config) IsBGPEnabled() bool {
	return c.BGP.Enabled
}

// GetBGPASN returns the BGP ASN if configured
func (c *Config) GetBGPASN() int {
	return c.BGP.ASN
}

// GetFRRSocketPath returns the FRR socket path
func (c *Config) GetFRRSocketPath() string {
	return c.FRR.SocketPath
}

// GetVtyshSocketArgs returns the extra arguments to pass to vtysh so it talks
// to the configured FRR socket (Finding 11). When the socket path is empty it
// returns nil and vtysh uses its default socket.
func (c *Config) GetVtyshSocketArgs() []string {
	if c.FRR.SocketPath == "" {
		return nil
	}
	return []string{"-s", c.FRR.SocketPath}
}

