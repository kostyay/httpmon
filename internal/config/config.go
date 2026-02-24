package config

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	configFile   = "config.json"
	oldTokenFile = "mcp-token"
)

// Config holds persistent httpmon settings stored in ~/.httpmon/config.json.
type Config struct {
	ProxyPort      int      `json:"proxy_port"`
	MCPEnabled     bool     `json:"mcp_enabled"`
	MCPAddr        string   `json:"mcp_addr"`
	MCPToken       string   `json:"mcp_token"`
	BufferSize     int      `json:"buffer_size"`
	ThrottlePreset string   `json:"throttle_preset"`
	ListMode       string   `json:"list_mode"`
	TreeGroupBy    string   `json:"tree_group_by"`
	ProtoPaths     []string `json:"proto_paths,omitempty"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		ProxyPort:  8080,
		MCPAddr:    "127.0.0.1:9551",
		BufferSize: 10000,
	}
}

// Load reads config.json from dataDir. Returns defaults if the file doesn't exist.
func Load(dataDir string) (*Config, error) {
	path := filepath.Join(dataDir, configFile)
	data, err := os.ReadFile(path) // #nosec G304 -- dataDir is user-provided config dir
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			cfg := DefaultConfig()
			return &cfg, nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	applyDefaults(&cfg)
	return &cfg, nil
}

// Save writes the config as indented JSON to dataDir/config.json.
func (c *Config) Save(dataDir string) error {
	if err := os.MkdirAll(dataDir, 0o750); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	path := filepath.Join(dataDir, configFile)
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

// LoadOrCreateToken ensures cfg.MCPToken is set. It migrates from the legacy
// mcp-token file if present, otherwise generates a new 32-byte hex token.
// The config is saved after the token is set.
func LoadOrCreateToken(cfg *Config, dataDir string) error {
	if cfg.MCPToken != "" {
		return nil
	}

	// Try migrating from legacy file.
	oldPath := filepath.Join(dataDir, oldTokenFile)
	if data, err := os.ReadFile(oldPath); err == nil { // #nosec G304
		tok := strings.TrimSpace(string(data))
		if tok != "" {
			cfg.MCPToken = tok
			_ = os.Remove(oldPath)
			return cfg.Save(dataDir)
		}
	}

	// Generate new token.
	var buf [32]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Errorf("generate token: %w", err)
	}
	cfg.MCPToken = hex.EncodeToString(buf[:])
	return cfg.Save(dataDir)
}

// ApplyFlags overrides config fields with explicitly-set CLI flags.
// Only flags the user actually passed on the command line are applied.
func ApplyFlags(cfg *Config, visit func(fn func(*flag.Flag))) {
	visit(func(f *flag.Flag) {
		switch f.Name {
		case "port":
			cfg.ProxyPort = mustInt(f.Value.String())
		case "buffer-size":
			cfg.BufferSize = mustInt(f.Value.String())
		case "mcp":
			cfg.MCPEnabled = f.Value.String() == "true"
		case "mcp-addr":
			cfg.MCPAddr = f.Value.String()
		case "throttle":
			cfg.ThrottlePreset = f.Value.String()
		}
	})
}

// applyDefaults fills zero-value fields with defaults so that older config
// files missing new fields still behave correctly.
func applyDefaults(cfg *Config) {
	d := DefaultConfig()
	if cfg.ProxyPort == 0 {
		cfg.ProxyPort = d.ProxyPort
	}
	if cfg.MCPAddr == "" {
		cfg.MCPAddr = d.MCPAddr
	}
	if cfg.BufferSize == 0 {
		cfg.BufferSize = d.BufferSize
	}
}

func mustInt(s string) int {
	v, _ := strconv.Atoi(s)
	return v
}
