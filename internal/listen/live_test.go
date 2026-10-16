package listen

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/samuelloranger/glim/internal/store"
)

// TestLiveProviders runs the whole pipeline against real endpoints. It is
// skipped unless GLIM_TEST_LLM_URL and GLIM_TEST_LLM_MODEL are set; the
// GLIM_TEST_TTS_* variables add speech, GLIM_TEST_PAGE names a preview folder
// (with index.html) to use instead of the built-in page, and
// GLIM_TEST_OUT keeps the result there.
func TestLiveProviders(t *testing.T) {
	llm := Endpoint{URL: os.Getenv("GLIM_TEST_LLM_URL"), Model: os.Getenv("GLIM_TEST_LLM_MODEL"), Key: os.Getenv("GLIM_TEST_LLM_KEY")}
	if llm.URL == "" || llm.Model == "" {
		t.Skip("set GLIM_TEST_LLM_URL and GLIM_TEST_LLM_MODEL to run against real providers")
	}
	set := Settings{
		LLM:    llm,
		TTS:    Endpoint{URL: os.Getenv("GLIM_TEST_TTS_URL"), Model: os.Getenv("GLIM_TEST_TTS_MODEL"), Key: os.Getenv("GLIM_TEST_TTS_KEY")},
		Voice:  os.Getenv("GLIM_TEST_TTS_VOICE"),
		Chrome: FindChrome(os.Getenv("GLIM_CHROME")),
	}
	src := os.Getenv("GLIM_TEST_PAGE")
	if src == "" {
		src = writePage(t)
	}
	st := store.New(t.TempDir(), "")
	res, err := st.Publish(src, "Live test", "", "", time.Hour, "")
	if err != nil {
		t.Fatal(err)
	}
	s := New(st, set, t.Logf)
	chatLog = t.Logf
	start := time.Now()
	if _, err := s.Start(res.Name, false); err != nil {
		t.Fatal(err)
	}
	var got Status
	for deadline := time.Now().Add(25 * time.Minute); time.Now().Before(deadline); time.Sleep(time.Second) {
		got = s.Status(res.Name)
		if got.State != "working" {
			break
		}
	}
	if got.State != "ready" {
		t.Fatalf("state %s: %s", got.State, got.Error)
	}
	r := got.Result
	t.Logf("took %s; rendered %v, %d pictures, language %q, voice %q, %.0f s of audio",
		time.Since(start).Round(time.Second), r.Rendered, r.Pictures, r.Language, r.Voice, r.Seconds)
	t.Logf("summary: %s", r.Summary)
	t.Logf("narration: %d words", len(splitWords(r.Narration)))
	if out := os.Getenv("GLIM_TEST_OUT"); out != "" {
		dir := filepath.Join(st.Dir(res.Name), Dir)
		for _, f := range []string{resultFile, audioFile} {
			if b, err := os.ReadFile(filepath.Join(dir, f)); err == nil {
				must(t, os.WriteFile(filepath.Join(out, f), b, 0o644))
			}
		}
	}
}

func splitWords(s string) []string {
	var out []string
	word := false
	start := 0
	for i, r := range s {
		space := r == ' ' || r == '\n' || r == '\t'
		if !space && !word {
			start, word = i, true
		} else if space && word {
			out = append(out, s[start:i])
			word = false
		}
	}
	if word {
		out = append(out, s[start:])
	}
	return out
}
