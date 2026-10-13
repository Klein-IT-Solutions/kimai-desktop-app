package voice

import (
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/gen2brain/malgo"
)

// MaxDuration caps a single recording.
const MaxDuration = 2 * time.Minute

// ErrNotRecording is returned by Stop without a running recording.
var ErrNotRecording = errors.New("no recording in progress")

// Recorder captures 16 kHz mono audio from the default input device.
// miniaudio picks PipeWire/PulseAudio/ALSA (Linux), CoreAudio or WASAPI.
type Recorder struct {
	// OnLevel receives the input level (0…1) about 15 times per second.
	OnLevel func(float64)
	// OnLimit is called when MaxDuration is reached; call Stop to collect the audio.
	OnLimit func()

	mu      sync.Mutex
	ctx     *malgo.AllocatedContext
	device  *malgo.Device
	samples []int16
	started time.Time
	pending []int16 // samples since the last level report
}

// Recording reports whether audio is being captured.
func (r *Recorder) Recording() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.device != nil
}

// Start opens the default microphone and begins recording.
func (r *Recorder) Start() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.device != nil {
		return nil
	}
	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return fmt.Errorf("Audiosystem nicht verfügbar: %v", err)
	}
	cfg := malgo.DefaultDeviceConfig(malgo.Capture)
	cfg.Capture.Format = malgo.FormatS16
	cfg.Capture.Channels = 1
	cfg.SampleRate = SampleRate
	cfg.Alsa.NoMMap = 1

	limitReported := false
	onData := func(_, input []byte, frames uint32) {
		chunk := make([]int16, frames)
		for i := range chunk {
			chunk[i] = int16(binary.LittleEndian.Uint16(input[i*2:]))
		}
		r.mu.Lock()
		r.samples = append(r.samples, chunk...)
		r.pending = append(r.pending, chunk...)
		var level float64
		report := len(r.pending) >= SampleRate/15
		if report {
			level = Level(r.pending)
			r.pending = r.pending[:0]
		}
		over := !limitReported && time.Since(r.started) > MaxDuration
		if over {
			limitReported = true
		}
		onLevel, onLimit := r.OnLevel, r.OnLimit
		r.mu.Unlock()
		if report && onLevel != nil {
			onLevel(level)
		}
		if over && onLimit != nil {
			go onLimit()
		}
	}
	device, err := malgo.InitDevice(ctx.Context, cfg, malgo.DeviceCallbacks{Data: onData})
	if err != nil {
		_ = ctx.Uninit()
		ctx.Free()
		return fmt.Errorf("Kein Mikrofon gefunden: %v", err)
	}
	r.samples = make([]int16, 0, SampleRate*10)
	r.pending = r.pending[:0]
	r.started = time.Now()
	if err := device.Start(); err != nil {
		device.Uninit()
		_ = ctx.Uninit()
		ctx.Free()
		return fmt.Errorf("Mikrofon konnte nicht gestartet werden: %v", err)
	}
	r.ctx, r.device = ctx, device
	return nil
}

// maxDrain bounds how long Stop waits for audio that is still in flight.
const maxDrain = 3 * time.Second

// Stop ends the recording and returns the captured samples. Sound servers
// may deliver capture data with a delay (large buffers, slow wake-up), so
// Stop first waits until the samples cover the recorded wall-clock time;
// otherwise the end of what was said would be cut off.
func (r *Recorder) Stop() ([]int16, error) {
	r.mu.Lock()
	device := r.device
	started := r.started
	r.mu.Unlock()
	if device == nil {
		return nil, ErrNotRecording
	}
	want := int(time.Since(started).Seconds() * SampleRate * 0.95)
	deadline := time.Now().Add(maxDrain)
	for time.Now().Before(deadline) {
		r.mu.Lock()
		have := len(r.samples)
		r.mu.Unlock()
		if have >= want {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	r.mu.Lock()
	device, ctx := r.device, r.ctx
	r.device, r.ctx = nil, nil
	r.mu.Unlock()
	if device == nil {
		return nil, ErrNotRecording
	}
	// Uninit waits for the audio thread, so take the samples afterwards.
	device.Uninit()
	_ = ctx.Uninit()
	ctx.Free()
	r.mu.Lock()
	samples := r.samples
	r.samples = nil
	r.mu.Unlock()
	return samples, nil
}
