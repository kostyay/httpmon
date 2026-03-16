package bodydecoder

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveBufCache_NoBufLock(t *testing.T) {
	dir := t.TempDir()
	paths, err := resolveBufCache(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(paths) != 0 {
		t.Errorf("expected no paths, got %v", paths)
	}
}

func TestResolveBufCache_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "buf.lock"), []byte("{{not yaml"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := resolveBufCache(dir)
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestResolveBufCache_ResolvesExistingDeps(t *testing.T) {
	// Set up a fake buf cache.
	cacheDir := t.TempDir()
	t.Setenv("BUF_CACHE_DIR", cacheDir)

	// Create a cached module directory.
	modDir := filepath.Join(cacheDir, "v3", "modules", "shake256",
		"buf.build", "acme", "validators", "abc123", "files")
	if err := os.MkdirAll(modDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Write a buf.lock referencing that module.
	protoDir := t.TempDir()
	lockContent := `version: v1
deps:
  - remote: buf.build
    owner: acme
    repository: validators
    commit: abc123
    digest: shake256:deadbeef
  - remote: buf.build
    owner: acme
    repository: missing
    commit: zzz999
    digest: shake256:cafebabe
`
	if err := os.WriteFile(filepath.Join(protoDir, "buf.lock"), []byte(lockContent), 0o600); err != nil {
		t.Fatal(err)
	}

	paths, err := resolveBufCache(protoDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should resolve the existing module, skip the missing one.
	if len(paths) != 1 {
		t.Fatalf("expected 1 path, got %d: %v", len(paths), paths)
	}
	if paths[0] != modDir {
		t.Errorf("path = %s, want %s", paths[0], modDir)
	}
}

func TestResolveBufCache_NoCacheDir(t *testing.T) {
	// Point BUF_CACHE_DIR to a non-existent directory.
	t.Setenv("BUF_CACHE_DIR", "/nonexistent/buf/cache")

	protoDir := t.TempDir()
	lockContent := `version: v1
deps:
  - remote: buf.build
    owner: acme
    repository: validators
    commit: abc123
    digest: shake256:deadbeef
`
	if err := os.WriteFile(filepath.Join(protoDir, "buf.lock"), []byte(lockContent), 0o600); err != nil {
		t.Fatal(err)
	}

	paths, err := resolveBufCache(protoDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(paths) != 0 {
		t.Errorf("expected no paths for missing cache, got %v", paths)
	}
}

func TestLoadProtoFiles_BufCacheIntegration(t *testing.T) {
	// Set up a fake buf cache with a shared proto.
	cacheDir := t.TempDir()
	t.Setenv("BUF_CACHE_DIR", cacheDir)

	modDir := filepath.Join(cacheDir, "v3", "modules", "shake256",
		"buf.build", "acme", "shared", "commit1", "files", "acme", "shared")
	if err := os.MkdirAll(modDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Write a shared proto into the cache.
	sharedProto := `syntax = "proto3";
package acme.shared;

message Timestamp {
  int64 seconds = 1;
}
`
	if err := os.WriteFile(filepath.Join(modDir, "timestamp.proto"), []byte(sharedProto), 0o600); err != nil {
		t.Fatal(err)
	}

	// Write the main proto that imports from the cache.
	protoDir := t.TempDir()
	mainProto := `syntax = "proto3";
package mypkg;

import "acme/shared/timestamp.proto";

message Event {
  string name = 1;
  acme.shared.Timestamp created_at = 2;
}

service EventService {
  rpc GetEvent(Event) returns (Event);
}
`
	if err := os.WriteFile(filepath.Join(protoDir, "event.proto"), []byte(mainProto), 0o600); err != nil {
		t.Fatal(err)
	}

	// Write buf.lock pointing to the cache.
	lockContent := `version: v1
deps:
  - remote: buf.build
    owner: acme
    repository: shared
    commit: commit1
    digest: shake256:deadbeef
`
	if err := os.WriteFile(filepath.Join(protoDir, "buf.lock"), []byte(lockContent), 0o600); err != nil {
		t.Fatal(err)
	}

	reg, errs := LoadProtoFiles([]string{protoDir})
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if !reg.HasMethods() {
		t.Fatal("expected methods to be loaded")
	}

	m, ok := reg.LookupMethod("/mypkg.EventService/GetEvent")
	if !ok {
		t.Fatal("GetEvent not found")
	}
	if string(m.Input().FullName()) != "mypkg.Event" {
		t.Errorf("input = %s, want mypkg.Event", m.Input().FullName())
	}
}
