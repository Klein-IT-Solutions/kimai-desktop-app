package voice

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// Local runs whisper.cpp's command-line tool (whisper-cli, formerly "main").
type Local struct {
	Binary string
	Model  string
	// Threads defaults to the number of CPUs (max. 8).
	Threads int
}

// Check verifies that binary and model exist.
func (l Local) Check() error {
	if l.Binary == "" {
		return errors.New("Pfad zu whisper.cpp (whisper-cli) fehlt")
	}
	bin, err := exec.LookPath(l.Binary)
	if err != nil {
		return fmt.Errorf("whisper.cpp nicht gefunden oder nicht ausführbar: %s", l.Binary)
	}
	if l.Model == "" {
		return errors.New("Pfad zum Whisper-Modell fehlt")
	}
	info, err := os.Stat(l.Model)
	if err != nil || info.IsDir() {
		return fmt.Errorf("Whisper-Modell nicht gefunden: %s", l.Model)
	}
	_ = bin
	return nil
}

// Transcribe implements Transcriber.
func (l Local) Transcribe(ctx context.Context, wav []byte, language string) (string, error) {
	if err := l.Check(); err != nil {
		return "", err
	}
	f, err := os.CreateTemp("", "kimai-tracker-*.wav")
	if err != nil {
		return "", err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(wav); err != nil {
		f.Close()
		return "", err
	}
	f.Close()

	threads := l.Threads
	if threads <= 0 {
		threads = min(runtime.NumCPU(), 8)
	}
	if language == "" {
		language = "auto"
	}
	cmd := exec.CommandContext(ctx, l.Binary,
		"-m", l.Model,
		"-f", f.Name(),
		"-l", language,
		"-t", fmt.Sprint(threads),
		"-nt", // no timestamps
		"-np", // no progress/info output
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		msg := strings.TrimSpace(lastLines(stderr.String(), 3))
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("whisper.cpp ist fehlgeschlagen: %s", msg)
	}
	text := cleanTranscript(stdout.String())
	if text == "" {
		return "", ErrNoSpeech
	}
	return text, nil
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, " ")
}
