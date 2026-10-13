package llm

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestParseBullets(t *testing.T) {
	cases := map[string]string{
		"- Angebot erstellt\n- Kunde angerufen":                    "- Angebot erstellt\n- Kunde angerufen",
		"Hier sind die Stichpunkte:\n\n* Angebot erstellt\n• Mail": "- Angebot erstellt\n- Mail",
		"1. Review\n2) Tests geschrieben\n":                        "- Review\n- Tests geschrieben",
		"**Zusammenfassung:**\n- **Meeting** vorbereitet":          "- Meeting vorbereitet",
		"Termin vorbereitet":                                       "- Termin vorbereitet",
		"- a\n- b\n- c\n- d\n- e\n- f\n- g":                        "- a\n- b\n- c\n- d\n- e\n- f",
		"":                                                         "",
	}
	for in, want := range cases {
		if got := ParseBullets(in); got != want {
			t.Errorf("ParseBullets(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAssetName(t *testing.T) {
	for _, c := range []struct{ os, arch, want string }{
		{"linux", "amd64", "llama-b100-bin-ubuntu-x64.tar.gz"},
		{"linux", "arm64", "llama-b100-bin-ubuntu-arm64.tar.gz"},
		{"darwin", "arm64", "llama-b100-bin-macos-arm64.tar.gz"},
		{"windows", "amd64", "llama-b100-bin-win-cpu-x64.zip"},
	} {
		if got, err := AssetName("b100", c.os, c.arch); err != nil || got != c.want {
			t.Errorf("AssetName(%s/%s) = %q, %v", c.os, c.arch, got, err)
		}
	}
	if _, err := AssetName("b1", "linux", "386"); err == nil {
		t.Error("386 should be unsupported")
	}
}

// tarGz builds an archive like llama.cpp's release (top-level llama-<tag>/).
func tarGz(t *testing.T, files map[string]string, links map[string]string) []byte {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg})
		io.WriteString(tw, body)
	}
	for name, target := range links {
		tw.WriteHeader(&tar.Header{Name: name, Linkname: target, Typeflag: tar.TypeSymlink})
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func TestResolveFromAPIPicksNewestReleaseWithAsset(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `[
			{"tag_name":"b300","draft":true,"assets":[{"name":"llama-b300-bin-ubuntu-x64.tar.gz","browser_download_url":"https://x/draft"}]},
			{"tag_name":"b299","assets":[{"name":"llama-b299-bin-macos-arm64.tar.gz","browser_download_url":"https://x/mac"}]},
			{"tag_name":"b298","assets":[{"name":"llama-b298-bin-ubuntu-x64.tar.gz","browser_download_url":"https://x/b298"},
			                             {"name":"llama-b298-bin-ubuntu-vulkan-x64.tar.gz","browser_download_url":"https://x/vulkan"}]},
			{"tag_name":"b100","assets":[{"name":"llama-b100-bin-ubuntu-x64.zip","browser_download_url":"https://x/old"}]}
		]`)
	}))
	defer srv.Close()
	a, err := ResolveFromAPI(context.Background(), srv.Client(), srv.URL, "linux", "amd64")
	if err != nil || a.Tag != "b298" || a.URL != "https://x/b298" {
		t.Fatalf("asset = %+v, %v", a, err)
	}
	a, err = ResolveFromAPI(context.Background(), srv.Client(), srv.URL, "darwin", "arm64")
	if err != nil || a.Tag != "b299" {
		t.Errorf("mac asset = %+v, %v", a, err)
	}
	if _, err := ResolveFromAPI(context.Background(), srv.Client(), srv.URL, "windows", "amd64"); err == nil {
		t.Error("windows asset should not be found")
	}
}

func TestResolveFallsBackToFeed(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusForbidden) }) // rate limited
	mux.HandleFunc("/releases.atom", func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `<feed><entry><link href="https://github.com/ggml-org/llama.cpp/releases/tag/b501"/></entry>
			<entry><link href="https://github.com/ggml-org/llama.cpp/releases/tag/b500"/></entry></feed>`)
	})
	// b501 is still uploading (no asset yet), b500 is complete.
	mux.HandleFunc("/releases/download/b500/llama-b500-bin-ubuntu-x64.tar.gz", func(w http.ResponseWriter, _ *http.Request) {})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	src := Source{APIURL: srv.URL + "/api", FeedURL: srv.URL + "/releases.atom", Base: srv.URL + "/releases"}
	a, err := Resolve(context.Background(), srv.Client(), src, "linux", "amd64")
	if err != nil || a.Tag != "b500" || !strings.HasSuffix(a.URL, "/download/b500/llama-b500-bin-ubuntu-x64.tar.gz") {
		t.Fatalf("asset = %+v, %v", a, err)
	}
}

func TestInstallFromMirror(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("asset naming tested for linux/amd64")
	}
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	archive := tarGz(t, map[string]string{
		"llama-" + FallbackTag + "/llama-server":      "#!/bin/sh\necho server\n",
		"llama-" + FallbackTag + "/libllama.so.0.0.1": "lib",
	}, map[string]string{"llama-" + FallbackTag + "/libllama.so": "libllama.so.0.0.1"})
	evil := tarGz(t, map[string]string{"../../escape": "x"}, nil)

	mux := http.NewServeMux()
	mux.HandleFunc("/good/download/"+FallbackTag+"/llama-"+FallbackTag+"-bin-ubuntu-x64.tar.gz", func(w http.ResponseWriter, _ *http.Request) { w.Write(archive) })
	mux.HandleFunc("/evil/download/"+FallbackTag+"/llama-"+FallbackTag+"-bin-ubuntu-x64.tar.gz", func(w http.ResponseWriter, _ *http.Request) { w.Write(evil) })
	srv := httptest.NewServer(mux)
	defer srv.Close()

	var calls int
	server, err := Install(context.Background(), nil, MirrorSource(srv.URL+"/good"), func(int64, int64) { calls++ })
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(server) != "llama-server" || calls == 0 {
		t.Fatalf("server = %q, progress calls = %d", server, calls)
	}
	if info, err := os.Stat(server); err != nil || info.Mode()&0o100 == 0 {
		t.Errorf("server not executable: %v %v", info, err)
	}
	if target, err := os.Readlink(filepath.Join(filepath.Dir(server), "libllama.so")); err != nil || target != "libllama.so.0.0.1" {
		t.Errorf("symlink = %q, %v", target, err)
	}
	if FindServer() != server {
		t.Errorf("FindServer = %q, want %q", FindServer(), server)
	}

	// "../../escape" is clamped into the target directory (no zip slip);
	// the archive is then rejected because llama-server is missing.
	if _, err := Install(context.Background(), nil, MirrorSource(srv.URL+"/evil"), nil); err == nil || !strings.Contains(err.Error(), "llama-server fehlt") {
		t.Errorf("evil archive: %v", err)
	}
	dir, _ := RuntimeDir()
	for _, p := range []string{filepath.Join(filepath.Dir(dir), "escape"), filepath.Join(filepath.Dir(filepath.Dir(dir)), "escape")} {
		if _, err := os.Stat(p); err == nil {
			t.Errorf("archive escaped its directory: %s", p)
		}
	}
}

// fakeLlamaServer mimics llama-server's /health and chat endpoint. It runs
// as a helper process: the test binary re-executed via a wrapper script.
func TestHelperLlamaServer(t *testing.T) {
	if os.Getenv("FAKE_LLAMA_SERVER") != "1" {
		t.Skip("helper process")
	}
	args := os.Args
	var port, model string
	for i, a := range args {
		switch a {
		case "--port":
			port = args[i+1]
		case "-m":
			model = args[i+1]
		}
	}
	if _, err := os.Stat(model); err != nil {
		fmt.Fprintln(os.Stderr, "error: failed to load model")
		os.Exit(1)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, `{"status":"ok"}`) })
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []struct{ Role, Content string }
		}
		json.NewDecoder(r.Body).Decode(&req)
		answer := "Hier sind die Stichpunkte:\n- " + strings.TrimPrefix(req.Messages[1].Content, "Notiz:\n") + "\n- Rolle: " + req.Messages[0].Role
		json.NewEncoder(w).Encode(map[string]any{"choices": []map[string]any{{"message": map[string]string{"content": answer}}}})
	})
	l, _ := net.Listen("tcp", "127.0.0.1:"+port)
	http.Serve(l, mux)
}

func fakeServerBinary(t *testing.T) string {
	if runtime.GOOS == "windows" {
		t.Skip("shell wrapper")
	}
	exe, _ := os.Executable()
	dir := t.TempDir()
	p := filepath.Join(dir, "llama-server")
	script := fmt.Sprintf("#!/bin/sh\nFAKE_LLAMA_SERVER=1 exec %q -test.run=TestHelperLlamaServer -- \"$@\"\n", exe)
	os.WriteFile(p, []byte(script), 0o755)
	return p
}

func TestServerSummarizeAndIdle(t *testing.T) {
	bin := fakeServerBinary(t)
	model := filepath.Join(t.TempDir(), "m.gguf")
	os.WriteFile(model, []byte("GGUF"), 0o644)
	old := IdleTimeout
	IdleTimeout = 400 * time.Millisecond
	defer func() { IdleTimeout = old }()

	s := NewServer()
	defer s.Stop()
	out, err := Summarize(context.Background(), s, bin, model, "  Angebot für ACME geschrieben  ")
	if err != nil {
		t.Fatal(err)
	}
	if out != "- Angebot für ACME geschrieben\n- Rolle: system" {
		t.Errorf("summary = %q", out)
	}
	if !s.Running() {
		t.Fatal("server should keep running between requests")
	}
	time.Sleep(900 * time.Millisecond)
	if s.Running() {
		t.Error("server should stop after the idle timeout")
	}
	// It starts again on demand.
	if _, err := Summarize(context.Background(), s, bin, model, "Test"); err != nil {
		t.Errorf("restart: %v", err)
	}
	if _, err := Summarize(context.Background(), s, bin, model, "   "); err != ErrEmpty {
		t.Errorf("empty: %v", err)
	}
}

func TestServerStartFailure(t *testing.T) {
	bin := fakeServerBinary(t)
	s := NewServer()
	defer s.Stop()
	if _, err := s.Chat(context.Background(), bin, "/nope.gguf", "s", "u", 10); err == nil {
		t.Error("missing model should fail")
	}
	broken := filepath.Join(t.TempDir(), "broken.gguf")
	os.WriteFile(broken, nil, 0o644)
	os.Remove(broken) // exists check passes for the app, the helper then fails to load
	model := filepath.Join(t.TempDir(), "m.gguf")
	os.WriteFile(model, []byte("GGUF"), 0o644)
	failing := filepath.Join(t.TempDir(), "llama-server")
	os.WriteFile(failing, []byte("#!/bin/sh\necho 'error: unknown model architecture' >&2\nexit 1\n"), 0o755)
	_, err := s.Chat(context.Background(), failing, model, "s", "u", 10)
	if err == nil || !strings.Contains(err.Error(), "unknown model architecture") {
		t.Errorf("start failure should include stderr: %v", err)
	}
}
