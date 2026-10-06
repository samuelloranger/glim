package main

import (
	"flag"
	"reflect"
	"strings"
	"testing"
)

func TestHumanBytes(t *testing.T) {
	cases := []struct {
		n    int64
		want string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KiB"},
		{1536, "1.5 KiB"},
		{1048576, "1.0 MiB"},
	}
	for _, c := range cases {
		if got := humanBytes(c.n); got != c.want {
			t.Errorf("humanBytes(%d) = %q, want %q", c.n, got, c.want)
		}
	}
}

func TestWriteQR(t *testing.T) {
	var b strings.Builder
	writeQR(&b, "https://glim.example.com/foo-abcd/")
	if b.Len() == 0 {
		t.Fatal("writeQR produced no output")
	}
}

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

func TestPublicHost(t *testing.T) {
	cases := map[string]string{
		"":                               "",
		"https://glim.example.com":       "glim.example.com",
		"https://glim.example.com:8443/": "glim.example.com:8443",
		"http://127.0.0.1:8787":          "127.0.0.1:8787",
	}
	for in, want := range cases {
		if got := publicHost(in); got != want {
			t.Errorf("publicHost(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSplitEntryEveryFlagBeforeAndAfter(t *testing.T) {
	fs, _ := newPublishFlags(0)
	values := valueFlagNames(fs)
	for _, want := range []string{"title", "project", "ttl", "name"} {
		if !values[want] {
			t.Errorf("%s should be a value flag", want)
		}
	}
	for _, b := range []string{"local", "qr"} {
		if values[b] {
			t.Errorf("%s is a bool flag, not a value flag", b)
		}
	}
	fs.VisitAll(func(f *flag.Flag) {
		var flagArgs []string
		if values[f.Name] {
			flagArgs = []string{"--" + f.Name, "some value"}
		} else {
			flagArgs = []string{"--" + f.Name}
		}
		inline := []string{"--" + f.Name + "=x"}
		cases := map[string][]string{
			"before":        append(append([]string{}, flagArgs...), "page.html"),
			"after":         append([]string{"page.html"}, flagArgs...),
			"inline-before": append(append([]string{}, inline...), "page.html"),
			"inline-after":  append([]string{"page.html"}, inline...),
		}
		for pos, args := range cases {
			entry, got := splitEntry(args)
			want := flagArgs
			if strings.HasPrefix(pos, "inline") {
				want = inline
			}
			if entry != "page.html" || !reflect.DeepEqual(got, want) {
				t.Errorf("--%s %s: entry=%q flags=%v, want page.html %v", f.Name, pos, entry, got, want)
			}
		}
	})
}

func TestSplitEntryNameFlag(t *testing.T) {
	entry, flags := splitEntry([]string{"--name", "foo", "page.html"})
	if entry != "page.html" || !reflect.DeepEqual(flags, []string{"--name", "foo"}) {
		t.Errorf("got %q %v", entry, flags)
	}
}

func TestPublishRejectsNonPositiveTTL(t *testing.T) {
	for _, ttl := range []string{"0s", "-1h"} {
		if err := cmdPublish([]string{"page.html", "--ttl", ttl}); err == nil || !strings.Contains(err.Error(), "ttl") {
			t.Errorf("publish --ttl %s: err = %v", ttl, err)
		}
	}
}

func TestExtendCmdRejectsNonPositiveTTL(t *testing.T) {
	for _, ttl := range []string{"0s", "-1h"} {
		if err := cmdExtend([]string{"x", ttl}); err == nil || !strings.Contains(err.Error(), "ttl") {
			t.Errorf("extend %s: err = %v", ttl, err)
		}
	}
}
