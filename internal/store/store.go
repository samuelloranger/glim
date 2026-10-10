package store

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/samuelloranger/glim/internal/render"
)

// Fingerprint is a cheap digest of which previews exist and when their
// manifests last changed; callers compare it to skip unchanged rescans.
func (s *Store) Fingerprint() (string, error) {
	entries, err := os.ReadDir(s.Root)
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	h := sha256.New()
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		info, err := os.Stat(filepath.Join(s.Root, e.Name(), ManifestFile))
		if err != nil {
			continue
		}
		fmt.Fprintf(h, "%s %d %d\n", e.Name(), info.ModTime().UnixNano(), info.Size())
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

type Store struct {
	Root    string
	BaseURL string
	Now     func() time.Time
	// OnRemove, if set, is called with the name of every preview Remove or GC
	// deletes (not for republishes, which keep the name).
	OnRemove func(name string)
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
	Locked  bool
}

// AllowedFileExts is the single place that decides which single-file entries
// may be published. Extend it here to allow more types.
// Non-HTML types are converted to index.html at publish time (see
// internal/render).
var AllowedFileExts = append([]string{".html", ".htm"}, render.Exts...)

// maxConvertBytes caps files read into memory for conversion to HTML.
const maxConvertBytes int64 = 20 << 20

// Directory publish limits, so pointing glim at a huge tree fails fast.
const (
	MaxPublishBytes int64 = 200 << 20
	MaxPublishFiles       = 5000
)

const (
	tmpPrefix = ".tmp-"
	oldPrefix = ".old-"
	staleAge  = time.Hour
)

func extAllowed(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	for _, a := range AllowedFileExts {
		if ext == a {
			return true
		}
	}
	return false
}

func (s *Store) Publish(entry, title, project, session string, ttl time.Duration, name string) (PublishResult, error) {
	return s.PublishLocked(entry, title, project, session, ttl, name, "")
}

// PublishLocked is Publish with an optional bcrypt passwordHash. An empty hash
// keeps whatever lock the preview already has when it is republished in place.
func (s *Store) PublishLocked(entry, title, project, session string, ttl time.Duration, name, passwordHash string) (PublishResult, error) {
	abs, err := filepath.Abs(entry)
	if err != nil {
		return PublishResult{}, err
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return PublishResult{}, fmt.Errorf("cannot read %s: %w", entry, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return PublishResult{}, fmt.Errorf("refusing to publish %s: it is a symlink", entry)
	}
	if strings.HasPrefix(filepath.Base(abs), ".") {
		return PublishResult{}, fmt.Errorf("refusing to publish %s: hidden (dot-prefixed) names are never published", entry)
	}
	if !info.IsDir() {
		if !info.Mode().IsRegular() {
			return PublishResult{}, fmt.Errorf("refusing to publish %s: not a regular file", entry)
		}
		if !extAllowed(abs) {
			return PublishResult{}, fmt.Errorf("refusing to publish %s: only %s files can be published as a single file", entry, strings.Join(AllowedFileExts, ", "))
		}
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
	if err := os.MkdirAll(s.Root, 0o755); err != nil {
		return PublishResult{}, err
	}

	// Build everything in a hidden temp dir; the live preview (if any) is only
	// touched by the final swap, so a failure leaves it intact.
	tmp, err := os.MkdirTemp(s.Root, tmpPrefix+name+"-")
	if err != nil {
		return PublishResult{}, err
	}
	ok := false
	defer func() {
		if !ok {
			os.RemoveAll(tmp)
		}
	}()
	if err := os.Chmod(tmp, 0o755); err != nil {
		return PublishResult{}, err
	}

	if info.IsDir() {
		idx, err := os.Lstat(filepath.Join(abs, "index.html"))
		if err != nil || !idx.Mode().IsRegular() {
			return PublishResult{}, fmt.Errorf("directory %s has no index.html", entry)
		}
		if err := copyTree(abs, tmp); err != nil {
			return PublishResult{}, err
		}
	} else if render.Supported(abs) {
		if err := publishConverted(abs, tmp, title, info.Size()); err != nil {
			return PublishResult{}, err
		}
	} else {
		if err := copyFile(abs, filepath.Join(tmp, "index.html")); err != nil {
			return PublishResult{}, err
		}
		if info.Size() <= maxConvertBytes {
			if page, err := os.ReadFile(abs); err == nil {
				copySiblingAssets(filepath.Dir(abs), tmp, page)
			}
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
		Version: newVersion(),
	}
	m.PasswordHash = passwordHash

	// Serialize with every other publish, lock, unlock, pin and extend of this
	// slug from here through the swap, so the manifest read below cannot be
	// stale and a concurrent change is never lost to the directory swap.
	unlock, err := s.lockSlug(name)
	if err != nil {
		return PublishResult{}, err
	}
	defer unlock()

	dir := filepath.Join(s.Root, name)
	if _, err := os.Lstat(dir); err == nil {
		old, err := readManifest(dir)
		switch {
		case err == nil:
			// A republish keeps the pin, and the title and project unless the
			// new call sets its own. An empty hash keeps the existing lock.
			m.Pinned = old.Pinned
			if m.Title == "" {
				m.Title = old.Title
			}
			if m.Project == "" {
				m.Project = old.Project
			}
			if passwordHash == "" {
				m.PasswordHash = old.PasswordHash
			}
		case passwordHash == "":
			// Fail closed: a live directory whose manifest cannot be read
			// must not be republished unlocked.
			return PublishResult{}, fmt.Errorf("cannot read the existing manifest of %s, refusing to republish it: %w", name, err)
		}
	} else if !os.IsNotExist(err) {
		return PublishResult{}, err
	}
	if err := writeManifest(tmp, m); err != nil {
		return PublishResult{}, err
	}

	var aside string
	if _, err := os.Lstat(dir); err == nil {
		aside = filepath.Join(s.Root, oldPrefix+name+"-"+filepath.Base(tmp)[len(tmpPrefix):])
		if err := os.Rename(dir, aside); err != nil {
			return PublishResult{}, err
		}
		// Rename keeps the directory's mtime; refresh it so GC's staleness
		// check cannot reap the aside copy while the swap is in flight.
		now := s.now()
		_ = os.Chtimes(aside, now, now)
	}
	if err := os.Rename(tmp, dir); err != nil {
		if aside != "" {
			_ = os.Rename(aside, dir)
		}
		return PublishResult{}, err
	}
	ok = true
	if aside != "" {
		os.RemoveAll(aside)
	}

	// GC takes the slug lock itself, so release it first.
	unlock()
	_, _ = s.GC()

	return PublishResult{Name: name, URL: s.url(name), Expires: m.Expires, Locked: m.Locked()}, nil
}

// publishConverted renders a non-HTML file into tmp/index.html and keeps the
// original next to it under its base name.
func publishConverted(abs, tmp, title string, size int64) error {
	base := filepath.Base(abs)
	if err := copyFile(abs, filepath.Join(tmp, base)); err != nil {
		return err
	}
	var data []byte
	if !isImage(base) {
		if size > maxConvertBytes {
			return fmt.Errorf("refusing to publish %s: larger than %d MB", base, maxConvertBytes>>20)
		}
		var err error
		if data, err = os.ReadFile(abs); err != nil {
			return err
		}
	}
	out, err := render.Convert(base, data, title)
	if err != nil {
		return fmt.Errorf("cannot publish %s: %w", base, err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "index.html"), out, 0o644); err != nil {
		return err
	}
	if ext := strings.ToLower(filepath.Ext(base)); ext == ".md" || ext == ".markdown" {
		copySiblingAssets(filepath.Dir(abs), tmp, out)
	}
	return nil
}

func isImage(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".avif", ".svg":
		return true
	}
	return false
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
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
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
	if !ValidName(name) {
		return fmt.Errorf("no such preview: %s", name)
	}
	unlock, err := s.lockSlug(name)
	if err != nil {
		return err
	}
	defer unlock()
	dir := filepath.Join(s.Root, name)
	if _, err := readManifest(dir); err != nil {
		return fmt.Errorf("no such preview: %s", name)
	}
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	s.removed(name)
	return nil
}

func (s *Store) removed(name string) {
	if s.OnRemove != nil {
		s.OnRemove(name)
	}
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
	if !ValidName(name) {
		return fmt.Errorf("no such preview: %s", name)
	}
	unlock, err := s.lockSlug(name)
	if err != nil {
		return err
	}
	defer unlock()
	dir := filepath.Join(s.Root, name)
	m, err := readManifest(dir)
	if err != nil || m.Expired(s.now()) {
		return fmt.Errorf("no such preview: %s", name)
	}
	m.Pinned = true
	return writeManifest(dir, m)
}

// SetPasswordHash locks the named preview behind a bcrypt hash, or unlocks it
// when hash is empty.
func (s *Store) SetPasswordHash(name, hash string) error {
	if !ValidName(name) {
		return fmt.Errorf("no such preview: %s", name)
	}
	unlock, err := s.lockSlug(name)
	if err != nil {
		return err
	}
	defer unlock()
	dir := filepath.Join(s.Root, name)
	m, err := readManifest(dir)
	if err != nil {
		return fmt.Errorf("no such preview: %s", name)
	}
	m.PasswordHash = hash
	return writeManifest(dir, m)
}

// Extend replaces a live preview's expiry with now+ttl. An expired preview is
// treated as gone even if GC has not removed it yet.
func (s *Store) Extend(name string, ttl time.Duration) error {
	if !ValidName(name) {
		return fmt.Errorf("no such preview: %s", name)
	}
	if err := ValidateTTL(ttl); err != nil {
		return err
	}
	if ttl > MaxTTL {
		return fmt.Errorf("ttl must be at most %s, got %s", MaxTTL, ttl)
	}
	unlock, err := s.lockSlug(name)
	if err != nil {
		return err
	}
	defer unlock()
	dir := filepath.Join(s.Root, name)
	m, err := readManifest(dir)
	if err != nil || m.Expired(s.now()) {
		return fmt.Errorf("no such preview: %s", name)
	}
	m.Expires = s.now().Add(ttl)
	return writeManifest(dir, m)
}

func (s *Store) DiskUsage() (int64, error) {
	var total int64
	err := filepath.WalkDir(s.Root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			// In-progress/aside publish dirs under the root are not previews.
			if path != s.Root && filepath.Dir(path) == s.Root && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
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
		if strings.HasPrefix(e.Name(), tmpPrefix) || strings.HasPrefix(e.Name(), oldPrefix) {
			if fi, err := e.Info(); err == nil && now.Sub(fi.ModTime()) > staleAge {
				if os.RemoveAll(dir) == nil {
					removed++
				}
			}
			continue
		}
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if s.reapExpired(e.Name(), now) {
			removed++
		}
	}
	return removed, nil
}

// reapExpired removes the named preview if it is expired, re-checking under
// the slug lock so an extend, pin or republish that landed after the caller's
// scan is not lost.
func (s *Store) reapExpired(name string, now time.Time) bool {
	dir := filepath.Join(s.Root, name)
	// Most previews are live: check without the lock, which is store-wide, so
	// a sweep does not make every publish, pin and extend queue behind it.
	if m, err := readManifest(dir); err != nil || !m.Expired(now) {
		return false
	}
	unlock, err := s.lockSlug(name)
	if err != nil {
		return false
	}
	defer unlock()
	m, err := readManifest(dir)
	if err != nil || !m.Expired(now) {
		return false
	}
	if os.RemoveAll(dir) != nil {
		return false
	}
	s.removed(name)
	return true
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

// copyTree copies src into dst, skipping dot-prefixed files and directories,
// failing on symlinks and other non-regular files, and enforcing the size and
// file-count caps.
func copyTree(src, dst string) error {
	var total int64
	var files int
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("refusing to publish: %s is a symlink or not a regular file", path)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		files++
		total += info.Size()
		if files > MaxPublishFiles {
			return fmt.Errorf("directory too large: more than %d files (at %s)", MaxPublishFiles, path)
		}
		if total > MaxPublishBytes {
			return fmt.Errorf("directory too large: more than %d MB (at %s)", MaxPublishBytes>>20, path)
		}
		return copyFile(path, target)
	})
}
