package voice

import (
	"os"
	"os/exec"
	"testing"
	"time"
)

// Records from the default input while playing a test file. Needs a running
// sound server whose default source hears the default sink (e.g. a PulseAudio
// null sink with its monitor as default source) and KIMAI_TRACKER_AUDIO_TEST
// pointing to a WAV file; skipped otherwise.
func TestRecorderCapturesAudio(t *testing.T) {
	file := os.Getenv("KIMAI_TRACKER_AUDIO_TEST")
	if file == "" {
		t.Skip("KIMAI_TRACKER_AUDIO_TEST not set")
	}
	var levels []float64
	r := &Recorder{OnLevel: func(l float64) { levels = append(levels, l) }}
	if err := r.Start(); err != nil {
		t.Fatal(err)
	}
	if !r.Recording() {
		t.Fatal("not recording")
	}
	if out, err := exec.Command("paplay", file).CombinedOutput(); err != nil {
		t.Fatalf("paplay: %v %s", err, out)
	}
	time.Sleep(300 * time.Millisecond)
	samples, err := r.Stop()
	if err != nil {
		t.Fatal(err)
	}
	secs := float64(len(samples)) / SampleRate
	t.Logf("captured %.1fs, %d level reports, overall level %.3f", secs, len(levels), Level(samples))
	if secs < 2 {
		t.Errorf("too short: %.1fs", secs)
	}
	if Level(samples) < 0.01 {
		t.Error("recording is silent")
	}
	if len(levels) < 10 {
		t.Errorf("only %d level reports", len(levels))
	}
	if out := os.Getenv("KIMAI_TRACKER_AUDIO_OUT"); out != "" {
		os.WriteFile(out, EncodeWAV(samples, SampleRate), 0o644)
	}
	if _, err := r.Stop(); err != ErrNotRecording {
		t.Errorf("second Stop: %v", err)
	}
}
