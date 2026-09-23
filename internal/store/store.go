package store

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Store struct {
	Root    string
	BaseURL string
	Now     func() time.Time
}

func New(root, baseURL string) *Store {
	return &Store{Root: root, BaseURL: baseURL, Now: time.Now}
}

func (s *Store) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

type PublishResult struct {
	Name    string
	URL     string
	Expires time.Time
}

func (s *Store) Publish(entry, title, project, session string, ttl time.Duration, name string) (PublishResult, error) {
	abs, err := filepath.Abs(entry)
	if err != nil {
		return PublishResult{}, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return PublishResult{}, fmt.Errorf("cannot read %s: %w", entry, err)
	}

	if name != "" {
		// Caller chose the slug (update-in-place at a stable URL). Validate
		// strictly before it becomes a path component under the store root.
		if !ValidName(name) {
			return PublishResult{}, fmt.Errorf("invalid preview name %q: use lowercase letters, digits and single hyphens only", name)
		}
	} else {
		label := title
		if label == "" {
			label = strings.TrimSuffix(filepath.Base(abs), filepath.Ext(abs))
		}
		name = NewName(label)
	}
	dir := filepath.Join(s.Root, name)
	// Clean slate so an update never leaks files from a prior version, and the
	// clock resets like a fresh publish.
	if err := os.RemoveAll(dir); err != nil {
		return PublishResult{}, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return PublishResult{}, err
	}

	if info.IsDir() {
		if _, err := os.Stat(filepath.Join(abs, "index.html")); err != nil {
			os.RemoveAll(dir)
			return PublishResult{}, fmt.Errorf("directory %s has no index.html", entry)
		}
		if err := copyTree(abs, dir); err != nil {
			os.RemoveAll(dir)
			return PublishResult{}, err
		}
	} else {
		if err := copyFile(abs, filepath.Join(dir, "index.html")); err != nil {
			os.RemoveAll(dir)
			return PublishResult{}, err
		}
	}

	created := s.now()
	m := Manifest{
		Name:    name,
		Title:   title,
		Project: project,
		Session: session,
		Created: created,
		Expires: created.Add(ttl),
	}
	if err := writeManifest(dir, m); err != nil {
		os.RemoveAll(dir)
		return PublishResult{}, err
	}

	_, _ = s.GC()

	return PublishResult{Name: name, URL: s.url(name), Expires: m.Expires}, nil
}

func (s *Store) url(name string) string {
	return strings.TrimRight(s.BaseURL, "/") + "/" + name + "/"
}

func (s *Store) URL(name string) string {
	return s.url(name)
}

func (s *Store) List() ([]Manifest, error) {
	entries, err := os.ReadDir(s.Root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Manifest
	now := s.now()
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		m, err := readManifest(filepath.Join(s.Root, e.Name()))
		if err != nil {
			continue
		}
		if m.Expired(now) {
			continue
		}
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Created.After(out[j].Created) })
	return out, nil
}

func (s *Store) Remove(name string) error {
	dir := filepath.Join(s.Root, name)
	if _, err := readManifest(dir); err != nil {
		return fmt.Errorf("no such preview: %s", name)
	}
	return os.RemoveAll(dir)
}

func (s *Store) Get(name string) (Manifest, error) {
	m, err := readManifest(filepath.Join(s.Root, name))
	if err != nil {
		return Manifest{}, fmt.Errorf("no such preview: %s", name)
	}
	return m, nil
}

// Live returns the manifest of a preview that may be served right now: the
// name is a valid slug, its manifest exists, and it has not expired.
func (s *Store) Live(name string) (Manifest, bool) {
	if !ValidName(name) {
		return Manifest{}, false
	}
	m, err := readManifest(filepath.Join(s.Root, name))
	if err != nil || m.Expired(s.now()) {
		return Manifest{}, false
	}
	return m, true
}

func (s *Store) Pin(name string) error {
	dir := filepath.Join(s.Root, name)
	m, err := readManifest(dir)
	if err != nil {
		return fmt.Errorf("no such preview: %s", name)
	}
	m.Pinned = true
	return writeManifest(dir, m)
}

func (s *Store) Extend(name string, ttl time.Duration) error {
	dir := filepath.Join(s.Root, name)
	m, err := readManifest(dir)
	if err != nil {
		return fmt.Errorf("no such preview: %s", name)
	}
	m.Expires = s.now().Add(ttl)
	return writeManifest(dir, m)
}

func (s *Store) DiskUsage() (int64, error) {
	var total int64
	err := filepath.WalkDir(s.Root, func(_ string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		total += info.Size()
		return nil
	})
	return total, err
}

func (s *Store) GC() (int, error) {
	entries, err := os.ReadDir(s.Root)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	now := s.now()
	removed := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(s.Root, e.Name())
		m, err := readManifest(dir)
		if err != nil {
			continue
		}
		if m.Expired(now) {
			if os.RemoveAll(dir) == nil {
				removed++
			}
		}
	}
	return removed, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(path, target)
	})
}
