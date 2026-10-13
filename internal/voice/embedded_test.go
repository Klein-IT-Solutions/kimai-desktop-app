package voice

import (
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const testModel = "../../third_party/whisper.cpp/models/for-tests-ggml-tiny.bin"

func sineWAV(seconds float64) []byte {
	n := int(seconds * SampleRate)
	s := make([]int16, n)
	for i := range s {
		s[i] = int16(8000 * math.Sin(2*math.Pi*440*float64(i)/SampleRate))
	}
	return EncodeWAV(s, SampleRate)
}

func TestDecodeWAV(t *testing.T) {
	got, err := decodeWAV(EncodeWAV([]int16{0, 16384, -32768}, SampleRate))
	if err != nil || len(got) != 3 || got[1] != 0.5 || got[2] != -1 {
		t.Fatalf("decode = %v, %v", got, err)
	}
	if _, err := decodeWAV([]byte("nope")); err == nil {
		t.Error("invalid data accepted")
	}
}

func TestEmbeddedTranscribe(t *testing.T) {
	if _, err := os.Stat(testModel); err != nil {
		t.Skip("whisper.cpp test model not available (run make whisper)")
	}
	old := EmbeddedIdleTimeout
	EmbeddedIdleTimeout = 300 * time.Millisecond
	defer func() { EmbeddedIdleTimeout = old }()

	e := Embedded{Model: testModel, Threads: 2}
	// The test model has random weights: it runs the full pipeline but
	// recognises nothing, so "no speech" is the expected outcome.
	for i := 0; i < 2; i++ {
		text, err := e.Transcribe(context.Background(), sineWAV(1.5), "de")
		if err != nil && !errors.Is(err, ErrNoSpeech) {
			t.Fatalf("run %d: %v", i, err)
		}
		t.Logf("run %d: %q", i, text)
	}
	engine.mu.Lock()
	loaded := engine.ctx != nil
	engine.mu.Unlock()
	if !loaded {
		t.Error("model should stay loaded between recordings")
	}
	time.Sleep(600 * time.Millisecond)
	engine.mu.Lock()
	loaded = engine.ctx != nil
	engine.mu.Unlock()
	if loaded {
		t.Error("model should be freed after the idle timeout")
	}
}

func TestEmbeddedErrors(t *testing.T) {
	if err := (Embedded{}).Check(); err == nil {
		t.Error("missing model accepted")
	}
	broken := filepath.Join(t.TempDir(), "broken.bin")
	os.WriteFile(broken, []byte("this is not a model"), 0o644)
	if _, err := (Embedded{Model: broken}).Transcribe(context.Background(), sineWAV(0.5), "de"); err == nil || errors.Is(err, ErrNoSpeech) {
		t.Errorf("broken model: %v", err)
	}
}
