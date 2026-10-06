package serve

import "testing"

func TestHTTPSNote(t *testing.T) {
	for _, d := range []string{"", "http://glim.example.com", "glim.example.com"} {
		if n := HTTPSNote(d); n != "" {
			t.Errorf("HTTPSNote(%q) = %q, want empty", d, n)
		}
	}
	if HTTPSNote("https://glim.example.com") == "" {
		t.Error("an https domain should produce a note")
	}
}
