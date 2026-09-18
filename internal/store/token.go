package store

import (
	"crypto/rand"
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

func NewName(label string) string {
	slug := Slugify(label)
	if slug == "" {
		slug = "preview"
	}
	return slug + "-" + randomSuffix()
}
