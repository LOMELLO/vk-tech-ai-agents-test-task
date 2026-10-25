package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
)

type Cache struct {
	dir string
}

type cacheEntry struct {
	ETag string `json:"etag"`
	Body string `json:"body"`
	Link string `json:"link"`
}

func NewCache(dir string) *Cache {
	return &Cache{dir: dir}
}

func (c *Cache) Load(url string) (cacheEntry, bool) {
	data, err := os.ReadFile(c.path(url))
	if err != nil {
		return cacheEntry{}, false
	}
	var entry cacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return cacheEntry{}, false
	}
	return entry, true
}

func (c *Cache) Store(url, etag string, body []byte, link string) error {
	if err := os.MkdirAll(c.dir, 0o755); err != nil {
		return err
	}
	entry := cacheEntry{ETag: etag, Body: string(body), Link: link}
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	path := c.path(url)
	tmp, err := os.CreateTemp(c.dir, ".entry-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func (c *Cache) path(url string) string {
	sum := sha256.Sum256([]byte(url))
	name := hex.EncodeToString(sum[:])
	return filepath.Join(c.dir, name+".json")
}
