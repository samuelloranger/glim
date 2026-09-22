package store

import (
	"crypto/rand"
	"regexp"
	"strings"
)

const suffixAlphabet = "abcdefghijkmnpqrstuvwxyz23456789"

const SuffixLen = 4

const maxSlugLen = 60

func randomSuffix() string {
	b := make([]byte, SuffixLen)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	out := make([]byte, SuffixLen)
	for i, v := range b {
		out[i] = suffixAlphabet[int(v)%len(suffixAlphabet)]
	}
	return string(out)
}

func Slugify(label string) string {
	var b strings.Builder
	prevHyphen := true
	for _, r := range strings.ToLower(label) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			prevHyphen = false
		} else if !prevHyphen {
			b.WriteByte('-')
			prevHyphen = true
		}
	}
	s := strings.Trim(b.String(), "-")
	if len(s) > maxSlugLen {
		s = strings.Trim(s[:maxSlugLen], "-")
	}
	return s
}

// nameRE matches the exact shape Slugify produces: lowercase letters and
// digits in hyphen-separated segments, no leading/trailing/double hyphens.
var nameRE = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// ValidName reports whether name is safe to use directly as a preview slug and
// on-disk directory component. It rejects anything outside the slug alphabet —
// empty, uppercase, dots, slashes, spaces, underscores, path traversal — so a
// caller-chosen name can never escape the store root.
func ValidName(name string) bool {
	return len(name) <= maxSlugLen && nameRE.MatchString(name)
}

func NewName(label string) string {
	slug := Slugify(label)
	if slug == "" {
		slug = "preview"
	}
	return slug + "-" + randomSuffix()
}
