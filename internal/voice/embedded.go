package voice

/*
#cgo CFLAGS: -I${SRCDIR}/../../third_party/whisper.cpp/include -I${SRCDIR}/../../third_party/whisper.cpp/ggml/include
#cgo LDFLAGS: -L${SRCDIR}/../../third_party/whisper.cpp/build/src -L${SRCDIR}/../../third_party/whisper.cpp/build/ggml/src
#cgo LDFLAGS: -lwhisper -lggml -lggml-cpu -lggml-base -lstdc++ -lm
#cgo linux LDFLAGS: -lpthread -ldl
#cgo windows LDFLAGS: -static
#cgo darwin LDFLAGS: -framework Accelerate -framework Foundation
#include <stdlib.h>
#include <whisper.h>

static void kt_silent_log(enum ggml_log_level level, const char *text, void *user_data) {}

static void kt_quiet(void) { whisper_log_set(kt_silent_log, NULL); }

static struct whisper_context *kt_load(const char *path) {
	struct whisper_context_params cp = whisper_context_default_params();
	cp.use_gpu = false;
	return whisper_init_from_file_with_params(path, cp);
}

static int kt_transcribe(struct whisper_context *ctx, const float *samples, int n, const char *lang, int threads) {
	struct whisper_full_params p = whisper_full_default_params(WHISPER_SAMPLING_GREEDY);
	p.n_threads        = threads;
	p.language         = lang;
	p.translate        = false;
	p.no_timestamps    = true;
	p.print_progress   = false;
	p.print_realtime   = false;
	p.print_special    = false;
	p.print_timestamps = false;
	p.suppress_blank   = true;
	return whisper_full(ctx, p, samples, n);
}
*/
import "C"

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
	"unsafe"
)

func init() { C.kt_quiet() }

// EmbeddedIdleTimeout frees the loaded model after this long without use.
var EmbeddedIdleTimeout = 5 * time.Minute

// Embedded transcribes with whisper.cpp compiled into the app (no external
// program needed). The model stays loaded between recordings.
type Embedded struct {
	Model   string
	Threads int
}

// Check verifies that the model exists.
func (e Embedded) Check() error {
	if e.Model == "" {
		return errors.New("Kein Whisper-Modell ausgewählt – bitte eines laden")
	}
	if info, err := os.Stat(e.Model); err != nil || info.IsDir() {
		return fmt.Errorf("Whisper-Modell nicht gefunden: %s", e.Model)
	}
	return nil
}

// Transcribe implements Transcriber.
func (e Embedded) Transcribe(ctx context.Context, wav []byte, language string) (string, error) {
	if err := e.Check(); err != nil {
		return "", err
	}
	samples, err := decodeWAV(wav)
	if err != nil {
		return "", err
	}
	threads := e.Threads
	if threads <= 0 {
		threads = min(runtime.NumCPU(), 8)
	}
	if language == "" {
		language = "auto"
	}
	type result struct {
		text string
		err  error
	}
	done := make(chan result, 1)
	go func() {
		text, err := engine.transcribe(e.Model, samples, language, threads)
		done <- result{text, err}
	}()
	select {
	case <-ctx.Done():
		// whisper.cpp cannot be interrupted here; the result is discarded.
		return "", ctx.Err()
	case r := <-done:
		if r.err != nil {
			return "", r.err
		}
		text := cleanTranscript(r.text)
		if text == "" {
			return "", ErrNoSpeech
		}
		return text, nil
	}
}

// decodeWAV reads 16-bit mono PCM (as produced by EncodeWAV) into float32.
func decodeWAV(wav []byte) ([]float32, error) {
	if len(wav) < 44 || string(wav[:4]) != "RIFF" || string(wav[8:12]) != "WAVE" {
		return nil, errors.New("ungültige Audiodaten")
	}
	// Find the data chunk (EncodeWAV writes it at offset 36).
	off := 12
	for off+8 <= len(wav) {
		id, size := string(wav[off:off+4]), int(binary.LittleEndian.Uint32(wav[off+4:off+8]))
		off += 8
		if id == "data" {
			end := min(off+size, len(wav))
			pcm := wav[off:end]
			out := make([]float32, len(pcm)/2)
			for i := range out {
				out[i] = float32(int16(binary.LittleEndian.Uint16(pcm[i*2:]))) / 32768
			}
			return out, nil
		}
		off += size
	}
	return nil, errors.New("keine Audiodaten gefunden")
}

// whisperEngine caches one loaded model.
type whisperEngine struct {
	mu    sync.Mutex
	ctx   *C.struct_whisper_context
	model string
	idle  *time.Timer
}

var engine = &whisperEngine{}

func (w *whisperEngine) transcribe(model string, samples []float32, language string, threads int) (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.ctx == nil || w.model != model {
		w.freeLocked()
		cpath := C.CString(model)
		ctx := C.kt_load(cpath)
		C.free(unsafe.Pointer(cpath))
		if ctx == nil {
			return "", fmt.Errorf("Whisper-Modell konnte nicht geladen werden: %s", model)
		}
		w.ctx, w.model = ctx, model
	}
	if w.idle != nil {
		w.idle.Stop()
	}
	// Restart the idle timer once this transcription has finished.
	defer func() { w.idle = time.AfterFunc(EmbeddedIdleTimeout, w.free) }()

	if len(samples) == 0 {
		return "", ErrNoSpeech
	}
	clang := C.CString(language)
	defer C.free(unsafe.Pointer(clang))
	// Copy into C memory: cgo must not keep Go pointers across the call's threads.
	n := len(samples)
	buf := (*[1 << 30]C.float)(C.malloc(C.size_t(n) * C.size_t(unsafe.Sizeof(C.float(0)))))[:n:n]
	defer C.free(unsafe.Pointer(&buf[0]))
	for i, s := range samples {
		buf[i] = C.float(s)
	}
	if rc := C.kt_transcribe(w.ctx, &buf[0], C.int(n), clang, C.int(threads)); rc != 0 {
		return "", fmt.Errorf("whisper.cpp-Transkription fehlgeschlagen (Code %d)", int(rc))
	}
	var sb strings.Builder
	for i := 0; i < int(C.whisper_full_n_segments(w.ctx)); i++ {
		sb.WriteString(C.GoString(C.whisper_full_get_segment_text(w.ctx, C.int(i))))
		sb.WriteByte(' ')
	}
	return sb.String(), nil
}

func (w *whisperEngine) free() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.freeLocked()
}

func (w *whisperEngine) freeLocked() {
	if w.ctx != nil {
		C.whisper_free(w.ctx)
		w.ctx = nil
		w.model = ""
	}
}

// ReleaseEmbedded frees the loaded model (e.g. on shutdown).
func ReleaseEmbedded() { engine.free() }
