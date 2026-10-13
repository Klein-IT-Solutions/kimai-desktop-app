package voice

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDownload(t *testing.T) {
	payload := append([]byte{0x6c, 0x6d, 0x67, 0x67}, make([]byte, 3<<20)...)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ggml-ok.bin":
			w.Header().Set("Content-Length", "3145732")
			w.Write(payload)
		case "/html.bin":
			w.Write([]byte("<html>login</html>"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	dir := t.TempDir()

	dest := filepath.Join(dir, "models", "ggml-ok.bin")
	var calls int
	var last int64
	err := Download(context.Background(), nil, srv.URL+"/ggml-ok.bin", dest, func(done, total int64) {
		calls++
		last = done
		if total != int64(len(payload)) {
			t.Errorf("total = %d", total)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(dest); err != nil || info.Size() != int64(len(payload)) {
		t.Fatalf("file = %v, %v", info, err)
	}
	if calls < 2 || last != int64(len(payload)) {
		t.Errorf("progress calls = %d, last = %d", calls, last)
	}

	if err := Download(context.Background(), nil, srv.URL+"/html.bin", filepath.Join(dir, "x.bin"), nil); err == nil || !strings.Contains(err.Error(), "kein whisper.cpp-Modell") {
		t.Errorf("non-model: %v", err)
	}
	if err := Download(context.Background(), nil, srv.URL+"/missing.bin", filepath.Join(dir, "y.bin"), nil); err == nil || !strings.Contains(err.Error(), "404") {
		t.Errorf("404: %v", err)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".download-") || e.Name() == "x.bin" || e.Name() == "y.bin" {
			t.Errorf("leftover file %s", e.Name())
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Download(ctx, nil, srv.URL+"/ggml-ok.bin", filepath.Join(dir, "z.bin"), nil); err == nil {
		t.Error("cancelled download should fail")
	}
}

func TestFindBinaryAndModelPath(t *testing.T) {
	data := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	t.Setenv("PATH", t.TempDir())
	if p := FindBinary(); p != "" && !strings.HasPrefix(p, "/usr/local/bin") && !strings.Contains(p, "brew") {
		t.Errorf("unexpected binary %q", p)
	}
	bin := filepath.Join(data, "kimai-tracker", "bin", "whisper-cli"+exeSuffix())
	os.MkdirAll(filepath.Dir(bin), 0o755)
	os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755)
	if p := FindBinary(); p != bin && !strings.HasPrefix(p, "/usr/local/bin") {
		t.Errorf("FindBinary = %q, want %q", p, bin)
	}
	m, _ := FindModel("small-q5_1")
	if p, _ := ModelPath(m); p != filepath.Join(data, "kimai-tracker", "models", "ggml-small-q5_1.bin") {
		t.Errorf("ModelPath = %q", p)
	}
}

func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}
