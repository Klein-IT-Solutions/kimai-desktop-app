// Package fetch downloads large files (models, runtimes) safely: to a
// temporary file, with progress, and only accepted when complete and of the
// expected type.
package fetch

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// Magic numbers of accepted file types.
var (
	MagicGGML = []byte{0x6c, 0x6d, 0x67, 0x67} // whisper.cpp ggml models
	MagicGGUF = []byte("GGUF")                 // llama.cpp models
	MagicGzip = []byte{0x1f, 0x8b}             // .tar.gz archives
	MagicZip  = []byte("PK\x03\x04")           // .zip archives
)

// File fetches url into dest, reporting progress. The data is written to a
// temporary file and only renamed once complete and starting with magic, so
// an interrupted download never leaves a broken file behind.
func File(ctx context.Context, client *http.Client, url, dest string, magic []byte, progress func(done, total int64)) error {
	if client == nil {
		client = &http.Client{}
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "kimai-tracker")
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("Download fehlgeschlagen: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Download fehlgeschlagen: HTTP %d", resp.StatusCode)
	}

	tmp, err := os.CreateTemp(filepath.Dir(dest), ".download-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())

	total := resp.ContentLength
	var done int64
	var head []byte
	buf := make([]byte, 256<<10)
	lastReport := int64(-1)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if len(head) < len(magic) {
				head = append(head, buf[:min(n, len(magic)-len(head))]...)
			}
			if _, err := tmp.Write(buf[:n]); err != nil {
				tmp.Close()
				return err
			}
			done += int64(n)
			if progress != nil && (done-lastReport >= 1<<20 || done == total) {
				progress(done, total)
				lastReport = done
			}
		}
		if errors.Is(rerr, io.EOF) {
			break
		}
		if rerr != nil {
			tmp.Close()
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("Download abgebrochen: %v", rerr)
		}
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if total > 0 && done != total {
		return fmt.Errorf("Download unvollständig (%d von %d Bytes)", done, total)
	}
	if !bytes.Equal(head, magic) {
		return ErrWrongType
	}
	return os.Rename(tmp.Name(), dest)
}

// ErrWrongType is returned when the downloaded data is not of the expected type.
var ErrWrongType = errors.New("Die heruntergeladene Datei hat ein unerwartetes Format")
