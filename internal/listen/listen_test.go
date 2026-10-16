package listen

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/samuelloranger/glim/internal/store"
)

// frame is one silent MPEG-1 layer III frame (128 kbps, 44.1 kHz, 417 bytes).
func frame(tag string) []byte {
	f := make([]byte, 417)
	copy(f, []byte{0xFF, 0xFB, 0x90, 0x00})
	copy(f[36:], tag)
	return f
}

// mp3File is what an encoder writes: an ID3v2 tag, a Xing header frame
// stating this file's own length, then n audio frames.
func mp3File(n int) []byte {
	b := []byte{'I', 'D', '3', 4, 0, 0, 0, 0, 0, 5, 1, 2, 3, 4, 5}
	b = append(b, frame("Xing")...)
	for i := 0; i < n; i++ {
		b = append(b, frame("")...)
	}
	return b
}

func TestMP3Frames(t *testing.T) {
	got, ok := mp3Frames(mp3File(3))
	if !ok {
		t.Fatal("not recognized as MP3")
	}
	if len(got) != 3*417 || bytes.Contains(got, []byte("Xing")) || bytes.Contains(got, []byte("ID3")) {
		t.Fatalf("got %d bytes", len(got))
	}
	if d := Duration(got); d < 0.07 || d > 0.08 { // 3 × 1152 / 44100
		t.Fatalf("duration %v", d)
	}
	if _, ok := mp3Frames([]byte(`{"error":"nope"}`)); ok {
		t.Fatal("JSON accepted as MP3")
	}
	if _, ok := mp3Frames([]byte("RIFF....WAVEfmt ")); ok {
		t.Fatal("WAV accepted as MP3")
	}
}

func TestChunks(t *testing.T) {
	text := strings.Repeat("One sentence here. ", 30) + "\n\n" + "Short paragraph.\n\n" + strings.Repeat("x", 50)
	cs := Chunks(text, 100)
	joined := strings.Join(cs, " ")
	for _, c := range cs {
		if len(c) > 100 {
			t.Fatalf("chunk of %d bytes: %q", len(c), c)
		}
	}
	if strings.Count(joined, "One sentence here.") != 30 || !strings.Contains(joined, "Short paragraph.") {
		t.Fatalf("text lost: %q", joined)
	}
	if cs := Chunks("é"+strings.Repeat("é", 60), 11); len(cs) == 0 || strings.Join(cs, "") != strings.Repeat("é", 61) {
		t.Fatalf("multibyte word split badly: %q", cs)
	}
	if len(Chunks("  \n\n ", 100)) != 0 {
		t.Fatal("blank text gave chunks")
	}
}

func TestParseScript(t *testing.T) {
	sc, err := parseScript("```json\n{\"language\":\"fr\",\"summary\":\"Un résumé.\",\"narration\":\"Bonjour.\"}\n```")
	if err != nil || sc.Language != "fr" || sc.Summary != "Un résumé." || sc.Narration != "Bonjour." {
		t.Fatalf("%+v %v", sc, err)
	}
	// Raw line breaks inside the strings, as models often send.
	sc, err = parseScript("{\n\"language\": \"en\",\n\"summary\": \"Short.\",\n\"narration\": \"First \\\"quoted\\\" part.\n\nSecond part.\"\n}")
	if err != nil || sc.Language != "en" || sc.Narration != "First \"quoted\" part.\n\nSecond part." {
		t.Fatalf("raw newlines: %+v %v", sc, err)
	}
	sc, err = parseScript("Just prose. Second. Third. Fourth.")
	if err != nil || sc.Narration != "Just prose. Second. Third. Fourth." || sc.Summary != "Just prose. Second. Third." {
		t.Fatalf("%+v %v", sc, err)
	}
}

// fakeLLM answers chat completions; reject lists request features it
// refuses with a 400, the way some providers do.
func fakeLLM(t *testing.T, reject ...string) (*httptest.Server, *[]map[string]any) {
	var mu sync.Mutex
	var seen []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer k" {
			http.Error(w, `{"error":{"message":"bad path or key"}}`, 404)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		seen = append(seen, body)
		mu.Unlock()
		raw, _ := json.Marshal(body)
		for _, feat := range reject {
			if strings.Contains(string(raw), feat) {
				w.WriteHeader(400)
				io.WriteString(w, `{"error":{"message":"`+feat+` is not supported"}}`)
				return
			}
		}
		io.WriteString(w, `{"choices":[{"message":{"content":"{\"language\":\"en-US\",\"summary\":\"A report.\",\"narration\":\"This page is a report.\"}"}}]}`)
	}))
	t.Cleanup(srv.Close)
	return srv, &seen
}

func TestDescribeFallsBack(t *testing.T) {
	p := Page{Title: "T", Text: "Body", Images: []Image{{Label: "Screenshot", MIME: "image/jpeg", Data: []byte{1}}}}
	srv, seen := fakeLLM(t, "json_object", "image_url")
	sc, err := Describe(context.Background(), srv.Client(), Endpoint{URL: srv.URL + "/v1/", Model: "m", Key: "k"}, p, "Be brief.")
	if err != nil || sc.Narration != "This page is a report." {
		t.Fatalf("%+v %v", sc, err)
	}
	if len(*seen) != 3 {
		t.Fatalf("%d attempts, want 3 (json+images, images, text only)", len(*seen))
	}
	sys := (*seen)[0]["messages"].([]any)[0].(map[string]any)["content"].(string)
	if !strings.Contains(sys, "Also: Be brief.") {
		t.Fatal("instructions missing from the prompt")
	}
}

func TestDescribeStopsOnAuthError(t *testing.T) {
	srv, seen := fakeLLM(t)
	_, err := Describe(context.Background(), srv.Client(), Endpoint{URL: srv.URL + "/v1", Model: "m", Key: "wrong"}, Page{Text: "x"}, "")
	if err == nil || !strings.Contains(err.Error(), "404") || len(*seen) != 0 {
		t.Fatalf("err %v, %d attempts", err, len(*seen))
	}
}

func TestSpeakJoinsChunks(t *testing.T) {
	var inputs []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Model, Input, Voice, Response_format string }
		_ = json.NewDecoder(r.Body).Decode(&body)
		if r.URL.Path != "/v1/audio/speech" || body.Voice != "v1" || body.Model != "tts" {
			w.WriteHeader(400)
			return
		}
		inputs = append(inputs, body.Input)
		w.Write(mp3File(2))
	}))
	defer srv.Close()
	text := strings.Repeat("A sentence to read. ", 400) // ~8 KB: two or three chunks
	var calls int
	out, err := Speak(context.Background(), srv.Client(), Endpoint{URL: srv.URL + "/v1", Model: "tts"}, "v1", text,
		func(done, total int) { calls++ })
	if err != nil {
		t.Fatal(err)
	}
	// One new Xing frame, then every chunk's audio frames without theirs.
	if len(inputs) < 2 || calls != len(inputs) || len(out) != frameLen(out)+len(inputs)*2*417 || bytes.Count(out, []byte("Xing")) != 1 {
		t.Fatalf("%d chunks, %d progress calls, %d bytes", len(inputs), calls, len(out))
	}
	off := xingOffset(out)
	if string(out[off:off+4]) != "Xing" || binary.BigEndian.Uint32(out[off+8:]) != uint32(len(inputs)*2) ||
		binary.BigEndian.Uint32(out[off+12:]) != uint32(len(out)) {
		t.Fatalf("Xing header %x", out[off:off+16])
	}
	if d, want := Duration(out), float64(len(inputs)*2*1152)/44100; d < want-0.001 || d > want+0.001 {
		t.Fatalf("duration %v, want %v (the header frame is not audio)", d, want)
	}
}

func newTestService(t *testing.T, set Settings) (*Service, *store.Store, string) {
	t.Helper()
	root := t.TempDir()
	st := store.New(root, "")
	src := writePage(t)
	res, err := st.Publish(src, "Report", "", "", time.Hour, "")
	if err != nil {
		t.Fatal(err)
	}
	if set.LLM.URL == "" {
		set.LLM = Endpoint{URL: "http://llm.invalid/v1", Model: "m"}
	}
	s := New(st, set, t.Logf)
	s.describe = func(ctx context.Context, p Page) (Script, error) {
		if !strings.Contains(p.Text, "Revenue rose") {
			return Script{}, errors.New("page text missing")
		}
		return Script{Language: "fr-CA", Summary: "Résumé.", Narration: "Bonjour. " + p.Title}, nil
	}
	s.speak = func(ctx context.Context, voice, text string, progress func(int, int)) ([]byte, error) {
		if voice != "ff_siwis" {
			return nil, errors.New("wrong voice " + voice)
		}
		progress(1, 1)
		f, _ := mp3Frames(mp3File(4))
		return f, nil
	}
	return s, st, res.Name
}

func wait(t *testing.T, s *Service, name string) Status {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if st := s.Status(name); st.State != "working" {
			return st
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("job never finished")
	return Status{}
}

func TestServiceMakesAndKeepsResult(t *testing.T) {
	s, st, name := newTestService(t, Settings{
		TTS: Endpoint{URL: "http://tts.invalid/v1", Model: "tts"}, Voice: "af_heart",
		Voices: map[string]string{"FR": "ff_siwis"},
	})
	if got := s.Status(name); got.State != "none" || !got.Speech {
		t.Fatalf("before: %+v", got)
	}
	if _, err := s.Start(name, false); err != nil {
		t.Fatal(err)
	}
	got := wait(t, s, name)
	if got.State != "ready" || got.Stale || got.Result == nil || !got.Result.Audio || got.Result.Voice != "ff_siwis" {
		t.Fatalf("after: %+v %+v", got, got.Result)
	}
	if got.Result.Narration != "Bonjour. Quarterly report" || got.Result.Rendered {
		t.Fatalf("result %+v", got.Result)
	}
	path, _, ok := s.Audio(name)
	if !ok || filepath.Dir(path) != filepath.Join(st.Dir(name), Dir) {
		t.Fatalf("audio %q %v", path, ok)
	}
	// A current result is reused unless asked again.
	s.describe = func(context.Context, Page) (Script, error) { t.Fatal("described twice"); return Script{}, nil }
	if again, _ := s.Start(name, false); again.State != "ready" {
		t.Fatalf("restart: %+v", again)
	}
	// A republish makes it stale, and removes it with the old folder.
	if _, err := st.Publish(writePage(t), "Report", "", "", time.Hour, name); err != nil {
		t.Fatal(err)
	}
	if got := s.Status(name); got.State != "none" {
		t.Fatalf("after republish: %+v", got)
	}
}

func TestServiceWithoutSpeechOrChrome(t *testing.T) {
	s, _, name := newTestService(t, Settings{})
	if _, err := s.Start(name, false); err != nil {
		t.Fatal(err)
	}
	got := wait(t, s, name)
	if got.State != "ready" || got.Speech || got.Result.Audio || got.Result.Narration == "" {
		t.Fatalf("%+v %+v", got, got.Result)
	}
	if _, _, ok := s.Audio(name); ok {
		t.Fatal("audio reported without a speech endpoint")
	}
}

func TestServiceFailureIsReported(t *testing.T) {
	s, _, name := newTestService(t, Settings{})
	s.describe = func(context.Context, Page) (Script, error) {
		return Script{}, errors.New("The model at api.example.com answered 500: overloaded")
	}
	s.Start(name, false)
	got := wait(t, s, name)
	if got.State != "failed" || got.Error != "The model at api.example.com answered 500: overloaded." {
		t.Fatalf("%+v", got)
	}
}

func TestServiceOff(t *testing.T) {
	st := store.New(t.TempDir(), "")
	s := New(st, Settings{}, nil)
	if s.Enabled() {
		t.Fatal("enabled without a model")
	}
	if _, err := s.Start("x", false); !errors.Is(err, ErrOff) {
		t.Fatalf("err = %v", err)
	}
	if _, err := New(st, Settings{LLM: Endpoint{URL: "u", Model: "m"}}, nil).Start("missing-ab12", false); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing preview: %v", err)
	}
}
