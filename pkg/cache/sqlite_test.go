package cache_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/oisee/vibing-steampunk/pkg/cache"
)

func TestNewSQLiteCacheCreatesParentDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "cache.db")
	config := cache.DefaultConfig()
	config.Type = "sqlite"
	config.Path = path

	c, err := cache.NewSQLiteCache(config)
	if err != nil {
		t.Fatalf("NewSQLiteCache failed: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	if _, err := os.Stat(filepath.Dir(path)); err != nil {
		t.Fatalf("cache parent directory was not created: %v", err)
	}
}
