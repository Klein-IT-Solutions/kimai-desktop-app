package voice

import (
	"bytes"
	"encoding/binary"
	"math"
)

// EncodeWAV encodes 16-bit mono PCM samples as a WAV file.
func EncodeWAV(samples []int16, sampleRate int) []byte {
	var b bytes.Buffer
	dataLen := uint32(len(samples) * 2)
	w := func(v any) { _ = binary.Write(&b, binary.LittleEndian, v) }
	b.WriteString("RIFF")
	w(uint32(36 + dataLen))
	b.WriteString("WAVEfmt ")
	w(uint32(16))             // fmt chunk size
	w(uint16(1))              // PCM
	w(uint16(1))              // mono
	w(uint32(sampleRate))     // sample rate
	w(uint32(sampleRate * 2)) // byte rate
	w(uint16(2))              // block align
	w(uint16(16))             // bits per sample
	b.WriteString("data")
	w(dataLen)
	w(samples)
	return b.Bytes()
}

// Level returns the RMS level of the samples in the range 0…1.
func Level(samples []int16) float64 {
	if len(samples) == 0 {
		return 0
	}
	var sum float64
	for _, s := range samples {
		v := float64(s) / 32768
		sum += v * v
	}
	return math.Sqrt(sum / float64(len(samples)))
}
