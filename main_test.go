package main

import (
	"reflect"
	"testing"
)

func TestSplitEntry(t *testing.T) {
	cases := []struct {
		name      string
		args      []string
		wantEntry string
		wantFlags []string
	}{
		{
			name:      "flags after path, value with spaces",
			args:      []string{"feat.html", "--title", "Rawkoon audiobooks new feature", "--ttl", "2h"},
			wantEntry: "feat.html",
			wantFlags: []string{"--title", "Rawkoon audiobooks new feature", "--ttl", "2h"},
		},
		{
			name:      "flags before path",
			args:      []string{"--title", "Diff review", "feat.html"},
			wantEntry: "feat.html",
			wantFlags: []string{"--title", "Diff review"},
		},
		{
			name:      "inline value form",
			args:      []string{"--title=Hi", "feat.html"},
			wantEntry: "feat.html",
			wantFlags: []string{"--title=Hi"},
		},
		{
			name:      "extra positional becomes flag arg (triggers usage error later)",
			args:      []string{"a.html", "b.html"},
			wantEntry: "a.html",
			wantFlags: []string{"b.html"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			entry, flags := splitEntry(c.args)
			if entry != c.wantEntry {
				t.Errorf("entry = %q, want %q", entry, c.wantEntry)
			}
			if !reflect.DeepEqual(flags, c.wantFlags) {
				t.Errorf("flags = %v, want %v", flags, c.wantFlags)
			}
		})
	}
}
