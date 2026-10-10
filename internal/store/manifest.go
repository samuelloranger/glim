package store

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const ManifestFile = ".glim.json"

// manifestTmpPrefix names the in-flight file writeManifest renames into place.
const manifestTmpPrefix = ManifestFile + ".tmp-"

type Manifest struct {
	Name    string    `json:"name"`
	Title   string    `json:"title,omitempty"`
	Project string    `json:"project,omitempty"`
	Session string    `json:"session,omitempty"`
	Created time.Time `json:"created"`
	Expires time.Time `json:"expires"`
	Pinned  bool      `json:"pinned,omitempty"`
	// Version identifies one publish of the content. Pin, extend and lock
	// rewrite the manifest but keep it, so it changes only on republish.
	Version string `json:"version,omitempty"`
	// PasswordHash is a bcrypt hash; when set the preview is locked behind it.
	PasswordHash string `json:"password_hash,omitempty"`
}

// Locked reports whether the preview requires a password.
func (m Manifest) Locked() bool { return m.PasswordHash != "" }

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
	// Write beside the manifest and rename it over, so a concurrent reader or
	// a crash sees either the old manifest or the new one, never a partial.
	f, err := os.CreateTemp(dir, manifestTmpPrefix)
	if err != nil {
		return err
	}
	tmp := f.Name()
	_, err = f.Write(data)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Chmod(tmp, 0o644)
	}
	if err == nil {
		err = os.Rename(tmp, filepath.Join(dir, ManifestFile))
	}
	if err != nil {
		os.Remove(tmp)
	}
	return err
}

// Stamp returns a token that changes whenever the preview's content is
// republished. Manifests written before Version existed fall back to Created.
func (m Manifest) Stamp() string {
	if m.Version != "" {
		return m.Version
	}
	return fmt.Sprintf("%d", m.Created.UnixNano())
}

func newVersion() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
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
