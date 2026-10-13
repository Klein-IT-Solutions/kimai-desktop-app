package voice

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

// DefaultAPIURL is OpenAI's API; any compatible service (Groq, a local
// faster-whisper server, …) can be configured instead.
const DefaultAPIURL = "https://api.openai.com/v1"

// DefaultAPIModel is used when no model is configured.
const DefaultAPIModel = "whisper-1"

// API calls an OpenAI-compatible /audio/transcriptions endpoint.
type API struct {
	BaseURL string
	APIKey  string
	Model   string
	HTTP    *http.Client
}

// Check verifies the configuration without sending audio.
func (a API) Check() error {
	u := strings.TrimSpace(a.BaseURL)
	if u == "" {
		return errors.New("API-URL fehlt")
	}
	if !strings.HasPrefix(u, "https://") && !strings.HasPrefix(u, "http://") {
		return errors.New("API-URL muss mit https:// beginnen")
	}
	return nil
}

func (a API) endpoint() string {
	u := strings.TrimRight(strings.TrimSpace(a.BaseURL), "/")
	if strings.HasSuffix(u, "/audio/transcriptions") {
		return u
	}
	return u + "/audio/transcriptions"
}

// Transcribe implements Transcriber.
func (a API) Transcribe(ctx context.Context, wav []byte, language string) (string, error) {
	if err := a.Check(); err != nil {
		return "", err
	}
	model := a.Model
	if model == "" {
		model = DefaultAPIModel
	}

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("file", "speech.wav")
	if err != nil {
		return "", err
	}
	if _, err := fw.Write(wav); err != nil {
		return "", err
	}
	_ = mw.WriteField("model", model)
	_ = mw.WriteField("response_format", "json")
	if language != "" && language != "auto" {
		_ = mw.WriteField("language", language)
	}
	if err := mw.Close(); err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.endpoint(), &body)
	if err != nil {
		return "", fmt.Errorf("ungültige API-URL: %v", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if a.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+a.APIKey)
	}
	client := a.HTTP
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("Transkriptions-API nicht erreichbar: %v", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	if resp.StatusCode != http.StatusOK {
		var e struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(data, &e)
		switch {
		case resp.StatusCode == http.StatusUnauthorized:
			return "", errors.New("API-Key ungültig (401)")
		case e.Error.Message != "":
			return "", fmt.Errorf("Transkriptions-API meldet Fehler %d: %s", resp.StatusCode, e.Error.Message)
		default:
			return "", fmt.Errorf("Transkriptions-API meldet Fehler %d", resp.StatusCode)
		}
	}
	var out struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		// Some servers answer with plain text.
		out.Text = string(data)
	}
	text := cleanTranscript(out.Text)
	if text == "" {
		return "", ErrNoSpeech
	}
	return text, nil
}
