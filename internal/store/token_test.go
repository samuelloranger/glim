package store

import (
	"strings"
	"testing"
)

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"Rawkoon audiobooks new feature": "rawkoon-audiobooks-new-feature",
		"  Hello, World!  ":               "hello-world",
		"multi   spaces\tand\nnewlines":   "multi-spaces-and-newlines",
		"Accents-and__underscores":        "accents-and-underscores",
		"!!!":                             "",
		"":                                "",
		"UPPER":                           "upper",
	}
	for in, want := range cases {
		if got := Slugify(in); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSlugifyCaps(t *testing.T) {
	long := strings.Repeat("a", 200)
	if got := Slugify(long); len(got) > maxSlugLen {
		t.Fatalf("Slugify long len = %d, want <= %d", len(got), maxSlugLen)
	}
}

func TestNewNameShapeAndUniqueness(t *testing.T) {
	const draws = 1000
	seen := make(map[string]bool)
	for i := 0; i < draws; i++ {
		name := NewName("Rawkoon audiobooks new feature")
		if !strings.HasPrefix(name, "rawkoon-audiobooks-new-feature-") {
			t.Fatalf("name %q missing readable prefix", name)
		}
		suffix := name[len("rawkoon-audiobooks-new-feature-"):]
		if len(suffix) != SuffixLen {
			t.Fatalf("name %q suffix len = %d, want %d", name, len(suffix), SuffixLen)
		}
		seen[name] = true
	}
	// The random suffix is collision-resistant, not collision-proof: over a
	// keyspace of len(alphabet)^SuffixLen, the birthday bound makes occasional
	// duplicates expected across many draws. Assert broad diversity instead of
	// absolute uniqueness so the test is deterministic.
	if ratio := float64(len(seen)) / draws; ratio < 0.9 {
		t.Fatalf("only %d/%d distinct names (%.2f), suffix diversity too low", len(seen), draws, ratio)
	}
}

func TestNewNameEmptyLabelFallsBack(t *testing.T) {
	name := NewName("!!!")
	if !strings.HasPrefix(name, "preview-") {
		t.Fatalf("empty label name = %q, want preview- prefix", name)
	}
}

func TestValidName(t *testing.T) {
	valid := []string{"dashboard-k3n7", "preview", "a-b-c", "abc123", "rawkoon-audiobooks-new-feature-k3n7"}
	for _, n := range valid {
		if !ValidName(n) {
			t.Errorf("ValidName(%q) = false, want true", n)
		}
	}
	invalid := []string{
		"", ".", "..", "../evil", "a/b", "a\\b", "UP", "Bad-Name",
		"-lead", "trail-", "a--b", "a.b", "a b", "a_b", strings.Repeat("a", maxSlugLen+1),
	}
	for _, n := range invalid {
		if ValidName(n) {
			t.Errorf("ValidName(%q) = true, want false", n)
		}
	}
}
