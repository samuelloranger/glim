package listen

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/samuelloranger/glim/internal/store"
)

// Dir is the folder inside a preview that holds its audio version. Dot
// segments are never served, and the folder goes away with the preview or
// its next republish.
const Dir = ".glim-listen"

const (
	resultFile = "result.json"
	audioFile  = "audio.mp3"
)

// Job steps, in order.
const (
	StepQueued = "queued"
	StepPage   = "page"
	StepScript = "script"
	StepAudio  = "audio"
)

// ErrOff means no model is configured.
var ErrOff = errors.New("listen is not set up")

// Settings configure a Service. TTS.URL empty means no audio is made.
type Settings struct {
	LLM          Endpoint
	TTS          Endpoint
	Voice        string
	Voices       map[string]string
	Chrome       string // resolved browser path; empty uses the HTML only
	Instructions string
}

// Result is a finished audio version.
type Result struct {
	Version   string    `json:"version"`
	Created   time.Time `json:"created"`
	Language  string    `json:"language,omitempty"`
	Summary   string    `json:"summary"`
	Narration string    `json:"narration"`
	Audio     bool      `json:"audio"`
	Seconds   float64   `json:"seconds,omitempty"`
	Model     string    `json:"model"`
	Voice     string    `json:"voice,omitempty"`
	// Rendered is false when the page was read from its HTML without a
	// browser; Pictures counts the screenshots and images the model saw.
	Rendered bool `json:"rendered"`
	Pictures int  `json:"pictures"`
}

// Status is where a preview's audio version stands. Result is the last one
// made, kept while a new one is being made or after one failed.
type Status struct {
	State  string  `json:"state"` // none, working, ready or failed
	Step   string  `json:"step,omitempty"`
	Done   int     `json:"done,omitempty"`
	Total  int     `json:"total,omitempty"`
	Error  string  `json:"error,omitempty"`
	Result *Result `json:"result,omitempty"`
	// Stale is true when the preview was republished after Result was made.
	Stale bool `json:"stale"`
	// Speech is false when the server makes no audio (no speech endpoint):
	// clients read the narration with the device's own voice instead.
	Speech bool `json:"speech"`
}

type job struct {
	version     string
	step        string
	done, total int
	err         string
	audio       []byte
}

// Service makes audio versions one at a time, in the background.
type Service struct {
	st     *store.Store
	set    Settings
	client *http.Client
	logf   func(string, ...any)

	mu    sync.Mutex
	jobs  map[string]*job
	slot  chan struct{}
	timer func() time.Time

	// render and describe are swapped in tests.
	render   func(ctx context.Context, chrome, dir, slug string) (Page, error)
	describe func(ctx context.Context, p Page) (Script, error)
	speak    func(ctx context.Context, voice, text string, progress func(int, int)) ([]byte, error)
}

// New returns a Service; it is off (Enabled false) unless set.LLM has a URL
// and a model.
func New(st *store.Store, set Settings, logf func(string, ...any)) *Service {
	s := &Service{st: st, set: set, client: &http.Client{}, logf: logf,
		jobs: map[string]*job{}, slot: make(chan struct{}, 1), timer: time.Now}
	if s.logf == nil {
		s.logf = func(string, ...any) {}
	}
	s.render = RenderPage
	s.describe = func(ctx context.Context, p Page) (Script, error) {
		return Describe(ctx, s.client, s.set.LLM, p, s.set.Instructions)
	}
	s.speak = func(ctx context.Context, voice, text string, progress func(int, int)) ([]byte, error) {
		return Speak(ctx, s.client, s.set.TTS, voice, text, progress)
	}
	return s
}

// Enabled reports whether a model is configured.
func (s *Service) Enabled() bool {
	return s != nil && s.set.LLM.URL != "" && s.set.LLM.Model != ""
}

// Speech reports whether audio is made (a speech endpoint is configured).
func (s *Service) Speech() bool { return s.Enabled() && s.set.TTS.URL != "" }

// Status reports where name's audio version stands.
func (s *Service) Status(name string) Status {
	m, _ := s.st.Live(name)
	st := Status{State: "none", Speech: s.Speech()}
	if r, ok := s.result(name); ok {
		st.Result = &r
		st.State = "ready"
		st.Stale = r.Version != m.Version
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if j := s.jobs[name]; j != nil {
		if j.err != "" {
			st.State, st.Error = "failed", j.err
		} else {
			st.State, st.Step, st.Done, st.Total = "working", j.step, j.done, j.total
		}
	}
	return st
}

// Start makes name's audio version in the background, unless one is being
// made or a current one exists and again is false.
func (s *Service) Start(name string, again bool) (Status, error) {
	if !s.Enabled() {
		return Status{}, ErrOff
	}
	m, ok := s.st.Live(name)
	if !ok {
		return Status{}, os.ErrNotExist
	}
	s.mu.Lock()
	j := s.jobs[name]
	busy := j != nil && j.err == ""
	if !busy && !again {
		if r, ok := s.result(name); ok && r.Version == m.Version {
			delete(s.jobs, name) // an earlier failure is moot
			s.mu.Unlock()
			return s.Status(name), nil
		}
	}
	if !busy {
		j = &job{version: m.Version, step: StepQueued}
		s.jobs[name] = j
		go s.run(name, j)
	}
	s.mu.Unlock()
	return s.Status(name), nil
}

// Audio returns the path of name's audio file and its result.
func (s *Service) Audio(name string) (string, Result, bool) {
	r, ok := s.result(name)
	if !ok || !r.Audio {
		return "", Result{}, false
	}
	p := filepath.Join(s.st.Dir(name), Dir, audioFile)
	if _, err := os.Stat(p); err != nil {
		return "", Result{}, false
	}
	return p, r, true
}

func (s *Service) result(name string) (Result, bool) {
	var r Result
	data, err := os.ReadFile(filepath.Join(s.st.Dir(name), Dir, resultFile))
	if err != nil || json.Unmarshal(data, &r) != nil {
		return Result{}, false
	}
	return r, true
}

func (s *Service) update(j *job, f func(j *job)) {
	s.mu.Lock()
	f(j)
	s.mu.Unlock()
}

func (s *Service) run(name string, j *job) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	select {
	case s.slot <- struct{}{}:
	case <-ctx.Done():
		s.fail(name, j, "Timed out waiting for another audio version to finish.")
		return
	}
	defer func() { <-s.slot }()

	r, err := s.make(ctx, name, j)
	if err != nil {
		s.logf("listen: %s: %v", name, err)
		s.fail(name, j, sentence(err.Error()))
		return
	}
	if m, ok := s.st.Live(name); !ok || m.Version != j.version {
		s.fail(name, j, "The preview changed while its audio version was being made. Try again.")
		return
	}
	if err := s.save(name, r, j.audio); err != nil {
		s.logf("listen: %s: save: %v", name, err)
		s.fail(name, j, "Couldn't save the audio version. Check the glim serve log.")
		return
	}
	s.mu.Lock()
	if s.jobs[name] == j {
		delete(s.jobs, name)
	}
	s.mu.Unlock()
}

func (s *Service) fail(name string, j *job, msg string) {
	s.update(j, func(j *job) { j.err = msg })
}

func (s *Service) make(ctx context.Context, name string, j *job) (Result, error) {
	s.update(j, func(j *job) { j.step = StepPage })
	dir := s.st.Dir(name)
	var p Page
	var err error
	if s.set.Chrome != "" {
		rctx, cancel := context.WithTimeout(ctx, 90*time.Second)
		p, err = s.render(rctx, s.set.Chrome, dir, name)
		cancel()
		if err != nil {
			s.logf("listen: %s: %v; reading the HTML instead", name, err)
		}
	}
	if s.set.Chrome == "" || err != nil {
		if p, err = StaticPage(dir); err != nil {
			return Result{}, err
		}
	}

	s.update(j, func(j *job) { j.step = StepScript })
	sc, err := s.describe(ctx, p)
	if err != nil {
		return Result{}, err
	}
	r := Result{Version: j.version, Created: s.timer().UTC(), Language: sc.Language, Summary: sc.Summary,
		Narration: sc.Narration, Model: s.set.LLM.Model, Rendered: p.Rendered, Pictures: len(p.Images)}
	if s.set.TTS.URL == "" {
		return r, nil
	}

	s.update(j, func(j *job) { j.step, j.total = StepAudio, len(Chunks(sc.Narration, ChunkChars)) })
	r.Voice = s.voiceFor(sc.Language)
	audio, err := s.speak(ctx, r.Voice, sc.Narration, func(done, total int) {
		s.update(j, func(j *job) { j.done, j.total = done, total })
	})
	if err != nil {
		return Result{}, err
	}
	r.Audio, r.Seconds = true, Duration(audio)
	j.audio = audio
	return r, nil
}

// voiceFor picks the voice mapped to the language's primary subtag, else the
// default.
func (s *Service) voiceFor(lang string) string {
	primary, _, _ := strings.Cut(strings.ToLower(strings.TrimSpace(lang)), "-")
	for k, v := range s.set.Voices {
		if strings.ToLower(k) == primary && v != "" {
			return v
		}
	}
	return s.set.Voice
}

// save writes the audio first and the result last, each by rename, so a
// reader never sees a result whose audio isn't there.
func (s *Service) save(name string, r Result, audio []byte) error {
	// A preview removed since the version check must not be recreated as a
	// folder holding only this.
	if _, err := os.Stat(filepath.Join(s.st.Dir(name), store.ManifestFile)); err != nil {
		return err
	}
	dir := filepath.Join(s.st.Dir(name), Dir)
	if err := os.Mkdir(dir, 0o755); err != nil && !os.IsExist(err) {
		return err
	}
	if r.Audio {
		if err := writeAtomic(dir, audioFile, audio); err != nil {
			return err
		}
	} else {
		os.Remove(filepath.Join(dir, audioFile))
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(dir, resultFile, data)
}

func writeAtomic(dir, name string, data []byte) error {
	f, err := os.CreateTemp(dir, name+".tmp-")
	if err != nil {
		return err
	}
	tmp := f.Name()
	_, err = f.Write(data)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Chmod(tmp, 0o644)
	}
	if err == nil {
		err = os.Rename(tmp, filepath.Join(dir, name))
	}
	if err != nil {
		os.Remove(tmp)
	}
	return err
}

// sentence ends an error message with a full stop for the UI.
func sentence(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "Something went wrong."
	}
	if !strings.HasSuffix(s, ".") && !strings.HasSuffix(s, "…") {
		s += "."
	}
	return s
}
