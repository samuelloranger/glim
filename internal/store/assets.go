package store

import (
	"bytes"
	"errors"
	"html"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
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

// htmlTag matches one real start tag, honouring quoted attribute values, so
// escaped text such as &lt;img src=x&gt; never matches.
var htmlTag = regexp.MustCompile(`<[a-zA-Z](?:"[^"]*"|'[^']*'|[^>"'])*>`)

// inertHTML matches regions whose contents are text, not markup: comments,
// code samples, scripts and styles.
// The opening tag of pre/code/script/style is kept (a script may carry src).
var inertHTML = regexp.MustCompile(`(?is)<!--.*?-->|<(?:pre|code|script|style)\b[^>]*>.*?</(?:pre|code|script|style)\s*>`)
var inertOpen = regexp.MustCompile(`(?is)^<(?:pre|code|script|style)\b[^>]*>`)

func stripInert(page []byte) []byte {
	return inertHTML.ReplaceAllFunc(page, func(m []byte) []byte {
		if bytes.HasPrefix(m, []byte("<!--")) {
			return nil
		}
		return inertOpen.Find(m)
	})
}

var schemeRe = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*:`)

// copySiblingAssets copies files referenced by relative src/href attributes in
// page from srcDir into the same relative location under dst. Only regular
// files reached without symlinks, inside srcDir, with no hidden path segment,
// within the size caps are copied; anything else (missing, directory, escape,
// oversized, already present) is skipped silently.
func copySiblingAssets(srcDir, dst string, page []byte) {
	var total int64
	seen := map[string]bool{}
	var refs [][][]byte
	for _, tag := range htmlTag.FindAll(stripInert(page), -1) {
		refs = append(refs, refAttr.FindAllSubmatch(tag, -1)...)
	}
	for _, m := range refs {
		ref := string(m[1]) + string(m[2]) + string(m[3])
		rel, ok := localRef(html.UnescapeString(ref))
		if !ok || seen[rel] {
			continue
		}
		seen[rel] = true
		fi, ok := plainFileInfo(srcDir, rel)
		size := int64(0)
		if ok {
			size = fi.Size()
		}
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
		n, err := copyAssetFile(filepath.Join(srcDir, rel), out, fi, maxAssetTotal-total)
		if err != nil {
			os.Remove(out)
			continue
		}
		total += n
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

// copyAssetFile copies src to out without following a symlink swapped in after
// the earlier Lstat (want), refusing anything that is not the same regular
// file, and never copying more than maxAssetBytes or budget bytes. It returns
// the bytes actually copied; on error the caller removes the partial out.
func copyAssetFile(src, out string, want os.FileInfo, budget int64) (int64, error) {
	in, err := os.OpenFile(src, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return 0, err
	}
	defer in.Close()
	fi, err := in.Stat()
	if err != nil {
		return 0, err
	}
	if !fi.Mode().IsRegular() || !os.SameFile(want, fi) {
		return 0, errors.New("asset changed or is not a regular file")
	}
	limit := maxAssetBytes
	if budget < limit {
		limit = budget
	}
	dst, err := os.OpenFile(out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return 0, err
	}
	n, err := io.Copy(dst, io.LimitReader(in, limit+1))
	if cerr := dst.Close(); err == nil {
		err = cerr
	}
	if err == nil && n > limit {
		err = errors.New("asset exceeds size cap")
	}
	return n, err
}

// plainFileInfo returns the Lstat info of root/rel when every component is a
// real (non-symlink) directory and the last is a regular file.
func plainFileInfo(root, rel string) (os.FileInfo, bool) {
	cur := root
	parts := strings.Split(rel, string(filepath.Separator))
	for i, part := range parts {
		cur = filepath.Join(cur, part)
		fi, err := os.Lstat(cur)
		if err != nil || fi.Mode()&os.ModeSymlink != 0 {
			return nil, false
		}
		if i == len(parts)-1 {
			return fi, fi.Mode().IsRegular()
		}
		if !fi.IsDir() {
			return nil, false
		}
	}
	return nil, false
}
