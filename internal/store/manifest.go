package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const ManifestFile = ".glim.json"

type Manifest struct {
	Name    string    `json:"name"`
	Title   string    `json:"title,omitempty"`
	Project string    `json:"project,omitempty"`
	Session string    `json:"session,omitempty"`
	Created time.Time `json:"created"`
	Expires time.Time `json:"expires"`
	Pinned  bool      `json:"pinned,omitempty"`
}

func (m Manifest) Expired(now time.Time) bool {
	if m.Pinned {
		return false
	}
	return now.After(m.Expires)
}

func writeManifest(dir string, m Manifest) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, ManifestFile), data, 0o644)
}

func readManifest(dir string) (Manifest, error) {
	var m Manifest
	data, err := os.ReadFile(filepath.Join(dir, ManifestFile))
	if err != nil {
		return m, err
	}
	err = json.Unmarshal(data, &m)
	return m, err
}
