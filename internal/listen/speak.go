package listen

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ChunkChars is the most text sent in one speech request; OpenAI's limit is
// 4096 characters, and every provider glim has seen accepts at least that.
const ChunkChars = 4000

// Speak reads text aloud through e's /audio/speech and returns one MP3. Long
// text is split into chunks at paragraph, then sentence boundaries; progress,
// if set, is called after each chunk.
func Speak(ctx context.Context, client *http.Client, e Endpoint, voice, text string, progress func(done, total int)) ([]byte, error) {
	chunks := Chunks(text, ChunkChars)
	if len(chunks) == 0 {
		return nil, errors.New("There is nothing to read aloud")
	}
	var out []byte
	for i, c := range chunks {
		body := map[string]any{"model": e.Model, "input": c, "response_format": "mp3"}
		if voice != "" {
			body["voice"] = voice
		}
		var raw []byte
		if err := postJSON(ctx, client, e, "/audio/speech", body, &raw); err != nil {
			return nil, err
		}
		frames, ok := mp3Frames(raw)
		if !ok {
			return nil, errors.New("The speech endpoint at " + hostOf(e.URL) + " didn't send MP3 audio")
		}
		out = append(out, frames...)
		if progress != nil {
			progress(i+1, len(chunks))
		}
	}
	return withXing(out), nil
}

// Chunks splits text into pieces of at most max bytes, preferring paragraph
// breaks, then sentence ends, then spaces.
func Chunks(text string, max int) []string {
	var out []string
	var cur strings.Builder
	flush := func() {
		if s := strings.TrimSpace(cur.String()); s != "" {
			out = append(out, s)
		}
		cur.Reset()
	}
	add := func(piece, sep string) {
		if cur.Len() > 0 && cur.Len()+len(sep)+len(piece) > max {
			flush()
		}
		if cur.Len() > 0 {
			cur.WriteString(sep)
		}
		cur.WriteString(piece)
	}
	for _, para := range strings.Split(text, "\n\n") {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}
		if len(para) <= max {
			add(para, "\n\n")
			continue
		}
		for _, sent := range sentences(para) {
			if len(sent) <= max {
				add(sent, " ")
				continue
			}
			for _, w := range hardSplit(sent, max) {
				add(w, " ")
			}
		}
	}
	flush()
	return out
}

// sentences splits after ., ! or ? (and their full-width forms) followed by
// a space.
func sentences(s string) []string {
	var out []string
	start := 0
	for i, r := range s {
		if r != '.' && r != '!' && r != '?' && r != '。' && r != '！' && r != '？' {
			continue
		}
		next := i + utf8.RuneLen(r)
		if next < len(s) {
			nr, _ := utf8.DecodeRuneInString(s[next:])
			if !unicode.IsSpace(nr) && r < 0x3000 {
				continue
			}
		}
		if p := strings.TrimSpace(s[start:next]); p != "" {
			out = append(out, p)
		}
		start = next
	}
	if p := strings.TrimSpace(s[start:]); p != "" {
		out = append(out, p)
	}
	return out
}

// hardSplit cuts s at spaces into pieces of at most max bytes, and mid-word
// (on a rune boundary) only when a single word is longer than max.
func hardSplit(s string, max int) []string {
	var out []string
	var cur strings.Builder
	for _, w := range strings.Fields(s) {
		for len(w) > max {
			cut := max
			for cut > 0 && !utf8Start(w[cut]) {
				cut--
			}
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
			out = append(out, w[:cut])
			w = w[cut:]
		}
		if cur.Len() > 0 && cur.Len()+1+len(w) > max {
			out = append(out, cur.String())
			cur.Reset()
		}
		if cur.Len() > 0 {
			cur.WriteByte(' ')
		}
		cur.WriteString(w)
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

// mp3Frames returns the audio frames of an MP3 file, without its ID3 tags or
// the Xing/Info/VBRI header frame encoders put first. That header states the
// length of its own file, so keeping it in a joined file makes players stop
// or seek wrongly; without it they measure the joined stream.
func mp3Frames(b []byte) ([]byte, bool) {
	if len(b) >= 10 && string(b[:3]) == "ID3" {
		size := int(b[6]&0x7f)<<21 | int(b[7]&0x7f)<<14 | int(b[8]&0x7f)<<7 | int(b[9]&0x7f)
		skip := 10 + size
		if b[5]&0x10 != 0 {
			skip += 10 // footer
		}
		if skip > len(b) {
			return nil, false
		}
		b = b[skip:]
	}
	if len(b) >= 128 && string(b[len(b)-128:len(b)-125]) == "TAG" {
		b = b[:len(b)-128]
	}
	start := -1
	for i := 0; i+4 <= len(b); i++ {
		if n := frameLen(b[i:]); n > 0 && (i+n == len(b) || i+n+4 <= len(b) && frameLen(b[i+n:]) > 0) {
			start = i
			break
		}
	}
	if start < 0 {
		return nil, false
	}
	b = b[start:]
	if n := frameLen(b); n > 0 && n <= len(b) && isHeaderFrame(b[:n]) {
		b = b[n:]
	}
	return b, true
}

// isHeaderFrame reports whether a frame carries a Xing, Info or VBRI header
// instead of audio.
func isHeaderFrame(f []byte) bool {
	return bytes.Contains(f, []byte("Xing")) || bytes.Contains(f, []byte("Info")) || bytes.Contains(f, []byte("VBRI"))
}

var (
	bitratesV1 = [16]int{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 0}
	bitratesV2 = [16]int{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160, 0}
	rates      = map[byte][3]int{3: {44100, 48000, 32000}, 2: {22050, 24000, 16000}, 0: {11025, 12000, 8000}}
)

// frameHeader parses the MPEG audio layer III frame header at the start of b.
func frameHeader(b []byte) (length, samples, rate int) {
	if len(b) < 4 || b[0] != 0xFF || b[1]&0xE0 != 0xE0 {
		return 0, 0, 0
	}
	version := (b[1] >> 3) & 3
	if version == 1 || (b[1]>>1)&3 != 1 { // reserved version, or not layer III
		return 0, 0, 0
	}
	bi, si := b[2]>>4, (b[2]>>2)&3
	if si == 3 {
		return 0, 0, 0
	}
	rate = rates[version][si]
	kbps := bitratesV2[bi]
	coef, samples := 72, 576
	if version == 3 {
		kbps, coef, samples = bitratesV1[bi], 144, 1152
	}
	if kbps == 0 {
		return 0, 0, 0
	}
	return coef*kbps*1000/rate + int((b[2]>>1)&1), samples, rate
}

func frameLen(b []byte) int {
	n, _, _ := frameHeader(b)
	return n
}

// xingOffset is where the Xing tag sits in a frame: after the 4-byte header
// and the side information, whose size depends on the version and on mono.
func xingOffset(h []byte) int {
	mono := (h[3]>>6)&3 == 3
	switch {
	case (h[1]>>3)&3 == 3 && mono:
		return 4 + 17
	case (h[1]>>3)&3 == 3:
		return 4 + 32
	case mono:
		return 4 + 9
	}
	return 4 + 17
}

// withXing puts a Xing header frame in front of joined audio frames, giving
// the true frame and byte counts. Without it a player guesses a VBR file's
// length from its first frame's bitrate, so it shows the wrong duration and
// seeks to the wrong place.
func withXing(frames []byte) []byte {
	if len(frames) < 4 {
		return frames
	}
	h := []byte{frames[0], frames[1] | 0x01, frames[2] &^ 0x02, frames[3]} // no CRC, no padding
	off := xingOffset(h)
	length := 0
	for bi := byte(1); bi < 15; bi++ {
		h[2] = h[2]&0x0F | bi<<4
		if n := frameLen(h); n >= off+16 {
			length = n
			break
		}
	}
	if length == 0 {
		return frames
	}
	count := 0
	for rest := frames; len(rest) >= 4; count++ {
		n := frameLen(rest)
		if n == 0 || n > len(rest) {
			break
		}
		rest = rest[n:]
	}
	x := make([]byte, length, length+len(frames))
	copy(x, h)
	copy(x[off:], "Xing")
	binary.BigEndian.PutUint32(x[off+4:], 0x3) // frame and byte counts present
	binary.BigEndian.PutUint32(x[off+8:], uint32(count))
	binary.BigEndian.PutUint32(x[off+12:], uint32(length+len(frames)))
	return append(x, frames...)
}

// Duration returns the length in seconds of a stream of MP3 frames, not
// counting a leading Xing/Info header frame.
func Duration(frames []byte) float64 {
	if n := frameLen(frames); n > 0 && n <= len(frames) && isHeaderFrame(frames[:n]) {
		frames = frames[n:]
	}
	secs := 0.0
	for len(frames) >= 4 {
		n, samples, rate := frameHeader(frames)
		if n == 0 || n > len(frames) {
			break
		}
		secs += float64(samples) / float64(rate)
		frames = frames[n:]
	}
	return secs
}
