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

	// BGP
	BGP     BGPConfig `yaml:"bgp,omitempty"`
	BGPMode string    `yaml:"bgp_mode,omitempty"` // "connected" or "dynamic"

	// Logging
	Logging LoggingConfig `yaml:"logging,omitempty"`

	// FRR
	FRR FRRConfig `yaml:"frr,omitempty"`

	// Runtime
	NodeIP         string `yaml:"node_ip,omitempty"`         // host node IP (optional, autodetect if empty)
	NodeName       string `yaml:"node_name,omitempty"`       // hostname or k8s node name
	KubeConfigPath string `yaml:"kubeconfig,omitempty"`      // path for out-of-cluster mode
}

// ServicesConfig contains service discovery configuration
type ServicesConfig struct {
	Namespaces []string `yaml:"namespaces"`
}

// BGPConfig contains BGP-specific configuration
type BGPConfig struct {
	Enabled bool `yaml:"enabled"`
	ASN     int  `yaml:"asn,omitempty"`
	ExcludedIPs []string `yaml:"excluded_ips,omitempty"` 
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
			ASN:     65000,
		},
		BGPMode: "connected",
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
		fmt.Printf("Warning: Config file %s not found, using defaults\n", configPath)
		return config, nil
	}

	data, err := ioutil.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %v", configPath, err)
	}

	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("failed to parse config file %s: %v", configPath, err)
	}

	// Validate
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %v", err)
	}

	return config, nil
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	if len(c.Services.Namespaces) == 0 {
		return fmt.Errorf("at least one namespace must be specified")
	}
	if c.LoopIntervalSeconds <= 0 {
		return fmt.Errorf("loop_interval_seconds must be positive")
	}
	if c.FRR.SocketPath == "" {
		return fmt.Errorf("frr.socket_path cannot be empty")
	}
	if c.BGPMode != "connected" && c.BGPMode != "dynamic" {
		return fmt.Errorf("invalid bgp_mode: %s (must be connected or dynamic)", c.BGPMode)
	}
	return nil
}

// ---------------------- Helper Methods ----------------------

// GetNodeName returns the configured NodeName, or hostname if empty
func (c *Config) GetNodeName() string {
	if c.NodeName != "" {
		return c.NodeName
	}
	hostname, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return hostname
}

// GetBGPMode returns the configured BGP mode ("connected" or "dynamic")
func (c *Config) GetBGPMode() string {
	return c.BGPMode
}

// GetLoopInterval returns the loop interval in seconds
func (c *Config) GetLoopInterval() int {
	if c.LoopIntervalSeconds <= 0 {
		return 30
	}
	return c.LoopIntervalSeconds
}

// GetNamespaces returns the list of namespaces to monitor
func (c *Config) GetNamespaces() []string {
	if len(c.Services.Namespaces) == 0 {
		return []string{"default"}
	}
	return c.Services.Namespaces
}

