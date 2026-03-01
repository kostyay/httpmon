package config

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestLoad_MissingFile(t *testing.T) {
	dir := t.TempDir()
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	d := DefaultConfig()
	if cfg.ProxyPort != d.ProxyPort {
		t.Errorf("ProxyPort = %d, want %d", cfg.ProxyPort, d.ProxyPort)
	}
	if cfg.MCPAddr != d.MCPAddr {
		t.Errorf("MCPAddr = %q, want %q", cfg.MCPAddr, d.MCPAddr)
	}
	if cfg.BufferSize != d.BufferSize {
		t.Errorf("BufferSize = %d, want %d", cfg.BufferSize, d.BufferSize)
	}
}

func TestSaveLoadRoundtrip(t *testing.T) {
	dir := t.TempDir()
	orig := &Config{
		ProxyPort:      9090,
		MCPEnabled:     true,
		MCPAddr:        "0.0.0.0:1234",
		MCPToken:       "tok123",
		BufferSize:     500,
		ThrottlePreset: "3g",
		ListMode:       "tree",
		TreeGroupBy:    "host",
	}
	if err := orig.Save(dir); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.ProxyPort != orig.ProxyPort || got.MCPEnabled != orig.MCPEnabled ||
		got.MCPAddr != orig.MCPAddr || got.MCPToken != orig.MCPToken ||
		got.BufferSize != orig.BufferSize || got.ThrottlePreset != orig.ThrottlePreset ||
		got.ListMode != orig.ListMode || got.TreeGroupBy != orig.TreeGroupBy ||
		!slices.Equal(got.ProtoPaths, orig.ProtoPaths) {
		t.Errorf("roundtrip mismatch:\n got %+v\nwant %+v", got, orig)
	}
}

func TestLoad_AppliesDefaults(t *testing.T) {
	dir := t.TempDir()
	// Write a config with only one field set; rest should get defaults.
	data, _ := json.Marshal(Config{MCPEnabled: true})
	if err := os.WriteFile(filepath.Join(dir, configFile), data, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ProxyPort != 8080 {
		t.Errorf("ProxyPort = %d, want 8080", cfg.ProxyPort)
	}
	if cfg.BufferSize != 10000 {
		t.Errorf("BufferSize = %d, want 10000", cfg.BufferSize)
	}
	if !cfg.MCPEnabled {
		t.Error("MCPEnabled should be true")
	}
}

func TestTokenMigration(t *testing.T) {
	dir := t.TempDir()
	oldTok := "legacy-token-value"
	if err := os.WriteFile(filepath.Join(dir, oldTokenFile), []byte(oldTok+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := &Config{}
	if err := LoadOrCreateToken(cfg, dir); err != nil {
		t.Fatalf("LoadOrCreateToken: %v", err)
	}
	if cfg.MCPToken != oldTok {
		t.Errorf("MCPToken = %q, want %q", cfg.MCPToken, oldTok)
	}
	// Old file should be removed.
	if _, err := os.Stat(filepath.Join(dir, oldTokenFile)); !os.IsNotExist(err) {
		t.Error("old mcp-token file should be deleted")
	}
	// Config should be persisted.
	loaded, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.MCPToken != oldTok {
		t.Errorf("persisted MCPToken = %q, want %q", loaded.MCPToken, oldTok)
	}
}

func TestLoadOrCreateToken_GeneratesNew(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{}
	if err := LoadOrCreateToken(cfg, dir); err != nil {
		t.Fatalf("LoadOrCreateToken: %v", err)
	}
	if len(cfg.MCPToken) != 64 { // 32 bytes = 64 hex chars
		t.Errorf("token length = %d, want 64", len(cfg.MCPToken))
	}
}

func TestLoadOrCreateToken_AlreadySet(t *testing.T) {
	cfg := &Config{MCPToken: "existing"}
	if err := LoadOrCreateToken(cfg, t.TempDir()); err != nil {
		t.Fatalf("LoadOrCreateToken: %v", err)
	}
	if cfg.MCPToken != "existing" {
		t.Error("should not overwrite existing token")
	}
}

func TestApplyFlags(t *testing.T) {
	cfg := DefaultConfig()

	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.Int("port", 8080, "")
	fs.Int("buffer-size", 10000, "")
	fs.Bool("mcp", false, "")
	fs.String("mcp-addr", "", "")
	fs.String("throttle", "", "")

	// Only set port and throttle.
	if err := fs.Parse([]string{"-port", "3128", "-throttle", "4g"}); err != nil {
		t.Fatal(err)
	}

	ApplyFlags(&cfg, fs.Visit)

	if cfg.ProxyPort != 3128 {
		t.Errorf("ProxyPort = %d, want 3128", cfg.ProxyPort)
	}
	if cfg.ThrottlePreset != "4g" {
		t.Errorf("ThrottlePreset = %q, want 4g", cfg.ThrottlePreset)
	}
	// Unset flags should keep defaults.
	if cfg.BufferSize != 10000 {
		t.Errorf("BufferSize = %d, want 10000 (unchanged)", cfg.BufferSize)
	}
	if cfg.MCPEnabled {
		t.Error("MCPEnabled should remain false")
	}
}

func TestApplyFlags_AllFlags(t *testing.T) {
	cfg := DefaultConfig()

	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.Int("port", 8080, "")
	fs.Int("buffer-size", 10000, "")
	fs.Bool("mcp", false, "")
	fs.String("mcp-addr", "", "")
	fs.String("throttle", "", "")

	if err := fs.Parse([]string{
		"-port", "9090",
		"-buffer-size", "500",
		"-mcp",
		"-mcp-addr", "0.0.0.0:5555",
		"-throttle", "3g",
	}); err != nil {
		t.Fatal(err)
	}

	ApplyFlags(&cfg, fs.Visit)

	if cfg.ProxyPort != 9090 {
		t.Errorf("ProxyPort = %d, want 9090", cfg.ProxyPort)
	}
	if cfg.BufferSize != 500 {
		t.Errorf("BufferSize = %d, want 500", cfg.BufferSize)
	}
	if !cfg.MCPEnabled {
		t.Error("MCPEnabled should be true")
	}
	if cfg.MCPAddr != "0.0.0.0:5555" {
		t.Errorf("MCPAddr = %q, want 0.0.0.0:5555", cfg.MCPAddr)
	}
	if cfg.ThrottlePreset != "3g" {
		t.Errorf("ThrottlePreset = %q, want 3g", cfg.ThrottlePreset)
	}
}

func TestLoad_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, configFile), []byte("{bad json"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestLoad_UnreadableFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, configFile)
	if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(path, 0o600) })

	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error for unreadable file")
	}
}

func TestProtoPaths_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultConfig()
	cfg.ProtoPaths = []string{"/home/user/protos", "/home/user/api/service.proto"}

	if err := cfg.Save(dir); err != nil {
		t.Fatalf("save: %v", err)
	}
	loaded, err := Load(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !slices.Equal(loaded.ProtoPaths, cfg.ProtoPaths) {
		t.Errorf("ProtoPaths = %v, want %v", loaded.ProtoPaths, cfg.ProtoPaths)
	}
}

func TestProtoPaths_OmittedWhenEmpty(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultConfig()
	if err := cfg.Save(dir); err != nil {
		t.Fatalf("save: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, configFile))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := raw["proto_paths"]; ok {
		t.Error("proto_paths should be omitted when empty")
	}
}

func TestProtoHosts_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultConfig()
	cfg.ProtoHosts = map[string]ProtoHostConfig{
		"api.example.com": {
			Paths:    []string{"/protos/api"},
			Includes: []string{"/protos/shared"},
		},
		"*.internal.io": {
			Paths: []string{"/protos/internal/service.proto"},
		},
	}

	if err := cfg.Save(dir); err != nil {
		t.Fatalf("save: %v", err)
	}
	loaded, err := Load(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded.ProtoHosts) != 2 {
		t.Fatalf("ProtoHosts len = %d, want 2", len(loaded.ProtoHosts))
	}
	api := loaded.ProtoHosts["api.example.com"]
	if !slices.Equal(api.Paths, []string{"/protos/api"}) {
		t.Errorf("api paths = %v", api.Paths)
	}
	if !slices.Equal(api.Includes, []string{"/protos/shared"}) {
		t.Errorf("api includes = %v", api.Includes)
	}
	internal := loaded.ProtoHosts["*.internal.io"]
	if !slices.Equal(internal.Paths, []string{"/protos/internal/service.proto"}) {
		t.Errorf("internal paths = %v", internal.Paths)
	}
}

func TestProtoHosts_OmittedWhenEmpty(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultConfig()
	if err := cfg.Save(dir); err != nil {
		t.Fatalf("save: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, configFile))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := raw["proto_hosts"]; ok {
		t.Error("proto_hosts should be omitted when empty")
	}
}

func TestProtoPaths_MissingFieldLoadsNil(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, configFile), []byte(`{"proxy_port":8080}`), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded.ProtoPaths) != 0 {
		t.Errorf("expected nil/empty ProtoPaths, got %v", loaded.ProtoPaths)
	}
}
