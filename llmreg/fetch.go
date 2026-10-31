package llmreg

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

var cacheDir string

func fetchJSON(url, cacheName string, refresh bool) ([]byte, error) {
	dir := cacheDir
	if dir == "" {
		base, err := os.UserCacheDir()
		if err != nil || base == "" {
			base = os.TempDir()
		}
		dir = filepath.Join(base, "gait")
	}
	cachePath := filepath.Join(dir, cacheName)

	if !refresh {
		buf, ok := readFreshCache(cachePath, 24*time.Hour)
		if ok {
			return buf, nil
		}
	}

	buf, err := download(url)
	if err != nil {
		buf, readErr := os.ReadFile(cachePath)
		if readErr != nil {
			return nil, err
		}

		return buf, nil
	}

	writeCache(cachePath, buf)
	return buf, nil
}

func readFreshCache(path string, ttl time.Duration) ([]byte, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, false
	}
	if time.Since(info.ModTime()) > ttl {
		return nil, false
	}
	buf, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	return buf, true
}

func writeCache(path string, buf []byte) {
	err := os.MkdirAll(filepath.Dir(path), 0o755)
	if err != nil {
		return
	}
	// Write atomically so a partial write never corrupts the cache.
	tmp := path + ".tmp"
	err = os.WriteFile(tmp, buf, 0o644)
	if err != nil {
		return
	}
	_ = os.Rename(tmp, path)
}

func download(url string) ([]byte, error) {
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %s", resp.Status)
	}
	return io.ReadAll(resp.Body)
}
