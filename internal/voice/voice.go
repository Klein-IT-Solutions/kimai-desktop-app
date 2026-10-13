// Package voice records speech from the microphone and transcribes it,
// either locally with whisper.cpp or through an OpenAI-compatible API.
package voice

import (
	"context"
	"errors"
	"regexp"
	"strings"
)

// SampleRate is the recording rate expected by Whisper models.
const SampleRate = 16000

// Transcriber turns a WAV recording into text.
type Transcriber interface {
	Transcribe(ctx context.Context, wav []byte, language string) (string, error)
}

// ErrNoSpeech is returned when the transcription is empty.
var ErrNoSpeech = errors.New("no speech recognised")

var (
	// whisper.cpp marks silence and non-speech like this.
	nonSpeech = regexp.MustCompile(`\[(BLANK_AUDIO|MUSIC|Musik|NOISE|SILENCE|Stille|no speech)[^\]]*\]|\((Musik|music|silence|Stille)\)`)
	// "[00:00:00.000 --> 00:00:02.000]" timestamps, in case they are printed.
	timestamps = regexp.MustCompile(`\[\d{2}:\d{2}:\d{2}\.\d{3} --> \d{2}:\d{2}:\d{2}\.\d{3}\]`)
	spaces     = regexp.MustCompile(`\s+`)
)

// cleanTranscript removes whisper markers and normalises whitespace.
func cleanTranscript(s string) string {
	s = timestamps.ReplaceAllString(s, " ")
	s = nonSpeech.ReplaceAllString(s, " ")
	return strings.TrimSpace(spaces.ReplaceAllString(s, " "))
}
