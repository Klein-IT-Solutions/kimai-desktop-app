package voice

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestEncodeWAV(t *testing.T) {
	wav := EncodeWAV([]int16{0, 1000, -1000}, SampleRate)
	if len(wav) != 44+6 || string(wav[:4]) != "RIFF" || string(wav[8:16]) != "WAVEfmt " || string(wav[36:40]) != "data" {
		t.Fatalf("bad header: %q", wav[:44])
	}
	if rate := binary.LittleEndian.Uint32(wav[24:28]); rate != SampleRate {
		t.Errorf("sample rate = %d", rate)
	}
	if n := binary.LittleEndian.Uint32(wav[40:44]); n != 6 {
		t.Errorf("data length = %d", n)
	}
	if v := int16(binary.LittleEndian.Uint16(wav[46:48])); v != 1000 {
		t.Errorf("sample = %d", v)
	}
}

func TestLevel(t *testing.T) {
	if Level(nil) != 0 || Level([]int16{0, 0}) != 0 {
		t.Error("silence should be 0")
	}
	if l := Level([]int16{32767, -32767}); l < 0.99 {
		t.Errorf("full scale = %v", l)
	}
}

func TestCleanTranscript(t *testing.T) {
	for in, want := range map[string]string{
		"  Hallo Welt.\n": "Hallo Welt.",
		"[BLANK_AUDIO]":   "",
		" [00:00:00.000 --> 00:00:02.000]  Guten Tag ": "Guten Tag",
		"(Musik) Meeting mit Kunde\n\n vorbereitet":    "Meeting mit Kunde vorbereitet",
	} {
		if got := cleanTranscript(in); got != want {
			t.Errorf("clean(%q) = %q, want %q", in, got, want)
		}
	}
}

// fakeWhisper writes a shell script that checks its arguments like whisper-cli.
func fakeWhisper(t *testing.T, output string, exit int) Local {
	if runtime.GOOS == "windows" {
		t.Skip("shell script")
	}
	dir := t.TempDir()
	model := filepath.Join(dir, "ggml-base.bin")
	os.WriteFile(model, []byte("model"), 0o644)
	script := filepath.Join(dir, "whisper-cli")
	body := `#!/bin/sh
echo "$@" > "` + dir + `/args"
while [ $# -gt 0 ]; do case "$1" in -f) test -s "$2" || exit 9; shift;; esac; shift; done
echo "whisper_init: loading model" >&2
printf '%s\n' "` + output + `"
exit ` + string(rune('0'+exit)) + `
`
	os.WriteFile(script, []byte(body), 0o755)
	return Local{Binary: script, Model: model, Threads: 2}
}

func TestLocalTranscribe(t *testing.T) {
	l := fakeWhisper(t, " Ticket 42 analysiert. [BLANK_AUDIO]", 0)
	text, err := l.Transcribe(context.Background(), EncodeWAV(make([]int16, 1600), SampleRate), "de")
	if err != nil || text != "Ticket 42 analysiert." {
		t.Fatalf("text = %q, err = %v", text, err)
	}
	args, _ := os.ReadFile(filepath.Join(filepath.Dir(l.Binary), "args"))
	for _, want := range []string{"-m " + l.Model, "-l de", "-t 2", "-nt", "-np"} {
		if !strings.Contains(string(args), want) {
			t.Errorf("args %q missing %q", args, want)
		}
	}
}

func TestLocalErrors(t *testing.T) {
	if _, err := (Local{}).Transcribe(context.Background(), nil, "de"); err == nil {
		t.Error("missing binary should fail")
	}
	l := fakeWhisper(t, "", 0)
	if _, err := l.Transcribe(context.Background(), EncodeWAV(make([]int16, 10), SampleRate), "de"); !errors.Is(err, ErrNoSpeech) {
		t.Errorf("empty output: %v", err)
	}
	l = fakeWhisper(t, "x", 3)
	if _, err := l.Transcribe(context.Background(), EncodeWAV(make([]int16, 10), SampleRate), "de"); err == nil || !strings.Contains(err.Error(), "loading model") {
		t.Errorf("failure should include stderr: %v", err)
	}
	l.Model = "/does/not/exist.bin"
	if err := l.Check(); err == nil {
		t.Error("missing model should fail Check")
	}
}

func TestAPITranscribe(t *testing.T) {
	var got struct {
		model, language, auth, format string
		fileLen                       int
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/audio/transcriptions" {
			http.NotFound(w, r)
			return
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Error(err)
		}
		got.model, got.language, got.format = r.FormValue("model"), r.FormValue("language"), r.FormValue("response_format")
		got.auth = r.Header.Get("Authorization")
		f, _, _ := r.FormFile("file")
		b, _ := io.ReadAll(f)
		got.fileLen = len(b)
		if got.auth != "Bearer sk-good" {
			w.WriteHeader(http.StatusUnauthorized)
			io.WriteString(w, `{"error":{"message":"Incorrect API key"}}`)
			return
		}
		io.WriteString(w, `{"text":" Code Review für Pull Request 17. "}`)
	}))
	defer srv.Close()

	wav := EncodeWAV(make([]int16, 160), SampleRate)
	a := API{BaseURL: srv.URL + "/v1/", APIKey: "sk-good"}
	text, err := a.Transcribe(context.Background(), wav, "de")
	if err != nil || text != "Code Review für Pull Request 17." {
		t.Fatalf("text = %q, err = %v", text, err)
	}
	if got.model != DefaultAPIModel || got.language != "de" || got.format != "json" || got.fileLen != len(wav) {
		t.Errorf("request = %+v", got)
	}

	a.APIKey = "sk-bad"
	if _, err := a.Transcribe(context.Background(), wav, "de"); err == nil || !strings.Contains(err.Error(), "401") {
		t.Errorf("bad key: %v", err)
	}
	a = API{BaseURL: srv.URL + "/v1/audio/transcriptions", APIKey: "sk-good", Model: "whisper-large-v3"}
	if _, err := a.Transcribe(context.Background(), wav, "auto"); err != nil || got.model != "whisper-large-v3" || got.language != "" {
		t.Errorf("full endpoint URL / custom model / auto language: %v %+v", err, got)
	}
	if err := (API{BaseURL: "ftp://x"}).Check(); err == nil {
		t.Error("bad scheme should fail")
	}
}
