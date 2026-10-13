package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// IdleTimeout stops the server after this long without requests, so the
// model does not occupy memory while it is not used.
var IdleTimeout = 5 * time.Minute

// StartTimeout bounds loading the model.
var StartTimeout = 90 * time.Second

// Server manages a llama-server child process bound to 127.0.0.1.
type Server struct {
	mu      sync.Mutex
	cmd     *exec.Cmd
	binary  string
	model   string
	baseURL string
	exited  chan struct{}
	idle    *time.Timer
	stderr  *tailBuffer
	http    *http.Client
}

// NewServer creates a stopped server.
func NewServer() *Server { return &Server{http: &http.Client{Timeout: 2 * time.Minute}} }

// Running reports whether the server process is up.
func (s *Server) Running() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cmd != nil
}

// ensure starts the server for binary/model unless it already runs with them.
func (s *Server) ensure(ctx context.Context, binary, model string) (string, error) {
	s.mu.Lock()
	if s.cmd != nil && s.binary == binary && s.model == model {
		url := s.baseURL
		s.resetIdleLocked()
		s.mu.Unlock()
		return url, nil
	}
	s.stopLocked()
	s.mu.Unlock()

	if _, err := os.Stat(binary); err != nil {
		return "", fmt.Errorf("llama.cpp nicht gefunden: %s", binary)
	}
	if _, err := os.Stat(model); err != nil {
		return "", fmt.Errorf("KI-Modell nicht gefunden: %s", model)
	}
	port, err := freePort()
	if err != nil {
		return "", err
	}
	threads := max(1, min(runtime.NumCPU()-1, 8))
	cmd := exec.Command(binary,
		"-m", model,
		"--host", "127.0.0.1",
		"--port", fmt.Sprint(port),
		"-c", "4096",
		"-t", fmt.Sprint(threads),
		"-np", "1",
	)
	// Release builds find their shared libraries via $ORIGIN; the library
	// path also covers builds without it.
	dir := filepath.Dir(binary)
	cmd.Env = append(os.Environ(),
		"LD_LIBRARY_PATH="+dir+string(os.PathListSeparator)+os.Getenv("LD_LIBRARY_PATH"),
		"DYLD_LIBRARY_PATH="+dir,
	)
	cmd.Dir = dir
	tail := &tailBuffer{max: 4096}
	cmd.Stdout, cmd.Stderr = io.Discard, tail
	setProcAttrs(cmd)
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("llama.cpp konnte nicht gestartet werden: %v", err)
	}
	exited := make(chan struct{})
	go func() { _ = cmd.Wait(); close(exited) }()

	url := fmt.Sprintf("http://127.0.0.1:%d", port)
	if err := waitHealthy(ctx, s.http, url, exited); err != nil {
		_ = cmd.Process.Kill()
		msg := strings.TrimSpace(lastLine(tail.String()))
		if msg != "" {
			return "", fmt.Errorf("%v (%s)", err, msg)
		}
		return "", err
	}

	s.mu.Lock()
	s.cmd, s.binary, s.model, s.baseURL, s.exited, s.stderr = cmd, binary, model, url, exited, tail
	s.resetIdleLocked()
	s.mu.Unlock()
	go func() {
		<-exited
		s.mu.Lock()
		if s.cmd == cmd {
			s.cmd = nil
		}
		s.mu.Unlock()
	}()
	return url, nil
}

func waitHealthy(ctx context.Context, client *http.Client, url string, exited chan struct{}) error {
	deadline := time.Now().Add(StartTimeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-exited:
			return errors.New("llama.cpp hat sich beim Laden des Modells beendet")
		case <-time.After(250 * time.Millisecond):
		}
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url+"/health", nil)
		resp, err := client.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
	}
	return errors.New("Zeitüberschreitung beim Laden des KI-Modells")
}

// Chat sends a chat completion request and returns the answer.
func (s *Server) Chat(ctx context.Context, binary, model, system, user string, maxTokens int) (string, error) {
	url, err := s.ensure(ctx, binary, model)
	if err != nil {
		return "", err
	}
	body, _ := json.Marshal(map[string]any{
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": user},
		},
		"temperature": 0.2,
		"max_tokens":  maxTokens,
		"stream":      false,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("KI-Anfrage fehlgeschlagen: %v", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("KI-Anfrage fehlgeschlagen: HTTP %d %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &out); err != nil || len(out.Choices) == 0 {
		return "", errors.New("unerwartete Antwort von llama.cpp")
	}
	s.mu.Lock()
	s.resetIdleLocked()
	s.mu.Unlock()
	return out.Choices[0].Message.Content, nil
}

// Stop terminates the server process.
func (s *Server) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopLocked()
}

func (s *Server) stopLocked() {
	if s.idle != nil {
		s.idle.Stop()
		s.idle = nil
	}
	if s.cmd == nil {
		return
	}
	cmd, exited := s.cmd, s.exited
	s.cmd = nil
	_ = terminate(cmd)
	select {
	case <-exited:
	case <-time.After(3 * time.Second):
		_ = cmd.Process.Kill()
	}
}

func (s *Server) resetIdleLocked() {
	if s.idle != nil {
		s.idle.Stop()
	}
	s.idle = time.AfterFunc(IdleTimeout, s.Stop)
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// tailBuffer keeps the last bytes written (for error messages).
type tailBuffer struct {
	mu  sync.Mutex
	buf []byte
	max int
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, p...)
	if len(t.buf) > t.max {
		t.buf = t.buf[len(t.buf)-t.max:]
	}
	return len(p), nil
}

func (t *tailBuffer) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.buf)
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return lines[len(lines)-1]
}
