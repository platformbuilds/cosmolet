// pkg/config/config.go
package config

import (
	"fmt"
	"io/ioutil"
	"os"

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
	ConfigPath string `yaml:"config_path,omitempty"`
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

	// Check if config file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// File doesn't exist, use defaults with warning
		fmt.Printf("Warning: Config file %s not found, using defaults\n", configPath)
		return config, nil
	}

	// Read config file
	data, err := ioutil.ReadFile(configPath)
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

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	// Validate services configuration
	if len(c.Services.Namespaces) == 0 {
		return fmt.Errorf("at least one namespace must be specified")
	}

	// Validate loop interval
	if c.LoopIntervalSeconds <= 0 {
		return fmt.Errorf("loop_interval_seconds must be positive")
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

// GetFRRConfigPath returns the FRR config path
func (c *Config) GetFRRConfigPath() string {
	return c.FRR.ConfigPath
}
