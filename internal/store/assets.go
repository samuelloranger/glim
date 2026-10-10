package store

import (
	"html"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	// maxAssetBytes caps one sibling asset copied alongside a single file.
	maxAssetBytes int64 = 25 << 20
	// maxAssetTotal caps all sibling assets copied for one publish.
	maxAssetTotal int64 = 100 << 20
)

// refAttr matches src and href attributes. It is a conservative regex rather
// than an HTML tokenizer (golang.org/x/net/html is not a dependency); a missed
// or spurious match only means an asset is not copied.
var refAttr = regexp.MustCompile(`(?i)\b(?:src|href)\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s"'>]+))`)

var schemeRe = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*:`)

// copySiblingAssets copies files referenced by relative src/href attributes in
// page from srcDir into the same relative location under dst. Only regular
// files reached without symlinks, inside srcDir, with no hidden path segment,
// within the size caps are copied; anything else (missing, directory, escape,
// oversized, already present) is skipped silently.
func copySiblingAssets(srcDir, dst string, page []byte) {
	var total int64
	seen := map[string]bool{}
	for _, m := range refAttr.FindAllSubmatch(page, -1) {
		ref := string(m[1]) + string(m[2]) + string(m[3])
		rel, ok := localRef(html.UnescapeString(ref))
		if !ok || seen[rel] {
			continue
		}
		seen[rel] = true
		size, ok := plainFileSize(srcDir, rel)
		if !ok || size > maxAssetBytes || total+size > maxAssetTotal {
			continue
		}
		out := filepath.Join(dst, rel)
		if _, err := os.Lstat(out); err == nil {
			continue // never overwrite the page or the kept original
		}
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			continue
		}
		if err := copyFile(filepath.Join(srcDir, rel), out); err != nil {
			os.Remove(out)
			continue
		}
		total += size
	}
}

// localRef turns an attribute value into a clean relative path, or reports
// false for anything that is not a plain local reference.
func localRef(ref string) (string, bool) {
	ref = strings.TrimSpace(ref)
	if ref == "" || strings.HasPrefix(ref, "/") || strings.HasPrefix(ref, `\`) || strings.HasPrefix(ref, "#") || schemeRe.MatchString(ref) {
		return "", false
	}
	if i := strings.IndexAny(ref, "?#"); i >= 0 {
		ref = ref[:i]
	}
	p, err := url.PathUnescape(ref)
	if err != nil || p == "" || strings.ContainsRune(p, 0) || strings.Contains(p, `\`) {
		return "", false
	}
	rel := filepath.Clean(filepath.FromSlash(p))
	if rel == "." || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	for _, seg := range strings.Split(rel, string(filepath.Separator)) {
		if strings.HasPrefix(seg, ".") {
			return "", false
		}
	}
	return rel, true
}

// plainFileSize reports the size of root/rel when every component is a real
// (non-symlink) directory and the last is a regular file.
func plainFileSize(root, rel string) (int64, bool) {
	cur := root
	parts := strings.Split(rel, string(filepath.Separator))
	for i, part := range parts {
		cur = filepath.Join(cur, part)
		fi, err := os.Lstat(cur)
		if err != nil || fi.Mode()&os.ModeSymlink != 0 {
			return 0, false
		}
		if i == len(parts)-1 {
			return fi.Size(), fi.Mode().IsRegular()
		}
		if !fi.IsDir() {
			return 0, false
		}
	}
	return 0, false
}
