package bodydecoder

import (
	"cmp"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"gopkg.in/yaml.v3"
)

type bufLockDep struct {
	Remote     string `yaml:"remote"`
	Owner      string `yaml:"owner"`
	Repository string `yaml:"repository"`
	Commit     string `yaml:"commit"`
}

type bufLockFile struct {
	Version string       `yaml:"version"`
	Deps    []bufLockDep `yaml:"deps"`
}

// resolveBufCache reads a buf.lock file from dir and returns import paths
// pointing into the local buf module cache for each dependency.
// Returns nil (no error) if buf.lock doesn't exist or can't be parsed.
func resolveBufCache(dir string) ([]string, error) {
	lockPath := filepath.Join(dir, "buf.lock")
	data, err := os.ReadFile(lockPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read buf.lock: %w", err)
	}

	var lock bufLockFile
	if err := yaml.Unmarshal(data, &lock); err != nil {
		return nil, fmt.Errorf("parse buf.lock: %w", err)
	}

	cacheRoot := bufCacheRoot()
	if cacheRoot == "" {
		return nil, nil
	}

	var paths []string
	for _, dep := range lock.Deps {
		// Cache layout: {cacheRoot}/v3/modules/shake256/{remote}/{owner}/{repository}/{commit}/files/
		p := filepath.Join(cacheRoot, "v3", "modules", "shake256",
			dep.Remote, dep.Owner, dep.Repository, dep.Commit, "files")
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			paths = append(paths, p)
		}
	}
	return paths, nil
}

// bufCacheRoot returns the buf cache directory.
// Respects BUF_CACHE_DIR env var, falls back to OS-specific default.
func bufCacheRoot() string {
	return cmp.Or(os.Getenv("BUF_CACHE_DIR"), defaultBufCacheDir())
}

func defaultBufCacheDir() string {
	switch runtime.GOOS {
	case "darwin", "linux":
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		return filepath.Join(home, ".cache", "buf")
	case "windows":
		if dir := os.Getenv("LOCALAPPDATA"); dir != "" {
			return filepath.Join(dir, "buf")
		}
		return ""
	default:
		return ""
	}
}
