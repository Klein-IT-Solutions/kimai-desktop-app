package app

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/klein-it-solutions/kimai-desktop-app/internal/config"
)

func newVoiceFixture(t *testing.T) (*VoiceService, config.MemorySecrets) {
	t.Helper()
	dir := t.TempDir()
	store, err := config.Open(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	secrets := config.MemorySecrets{}
	conn := NewConnection(store, secrets, &config.FileSecrets{Path: filepath.Join(dir, "secrets.json")})
	return NewVoiceService(store, conn, nil), secrets
}

func TestVoiceSettings(t *testing.T) {
	v, secrets := newVoiceFixture(t)
	if s := v.GetSettings(); s.Engine != "" || s.APIModel != "whisper-1" || s.Language != "de" {
		t.Fatalf("defaults = %+v", s)
	}
	if _, err := v.SaveSettings(VoiceSettingsInput{Engine: "magic"}); err == nil {
		t.Error("unknown engine accepted")
	}
	if _, err := v.SaveSettings(VoiceSettingsInput{Engine: "api", PushToTalk: "V"}); err == nil {
		t.Error("shortcut without modifier accepted")
	}

	s, err := v.SaveSettings(VoiceSettingsInput{
		Engine: "api", APIURL: " https://api.groq.com/openai/v1 ", APIModel: "whisper-large-v3",
		APIKey: " gsk-123 ", PushToTalk: "super+shift+v",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !s.HasAPIKey || secrets[config.SecretTranscriptionKey] != "gsk-123" || s.APIURL != "https://api.groq.com/openai/v1" {
		t.Errorf("settings = %+v, key = %q", s, secrets[config.SecretTranscriptionKey])
	}
	if got := v.store.Get().Voice.PushToTalk; got != "Super+Shift+V" {
		t.Errorf("push-to-talk = %q", got)
	}
	if msg, err := v.CheckSettings(); err != nil || msg == "" {
		t.Errorf("check api = %q, %v", msg, err)
	}

	// An empty key keeps the stored one.
	if _, err := v.SaveSettings(VoiceSettingsInput{Engine: "api", APIURL: "https://api.openai.com/v1"}); err != nil || secrets[config.SecretTranscriptionKey] != "gsk-123" {
		t.Errorf("key lost: %v", err)
	}
}

func TestVoiceCheckLocal(t *testing.T) {
	v, _ := newVoiceFixture(t)
	if _, err := v.SaveSettings(VoiceSettingsInput{Engine: "local", WhisperModel: "/nope/model.bin"}); err != nil {
		t.Fatal(err)
	}
	if _, err := v.CheckSettings(); err == nil {
		t.Error("missing model should fail (embedded engine)")
	}
	model0 := filepath.Join(t.TempDir(), "ggml-base.bin")
	os.WriteFile(model0, []byte("x"), 0o644)
	if _, err := v.SaveSettings(VoiceSettingsInput{Engine: "local", WhisperModel: model0}); err != nil {
		t.Fatal(err)
	}
	if msg, err := v.CheckSettings(); err != nil || msg == "" {
		t.Errorf("embedded engine with model: %q, %v", msg, err)
	}
	if _, err := v.SaveSettings(VoiceSettingsInput{Engine: "local", WhisperBinary: "/nope/whisper-cli", WhisperModel: model0}); err != nil {
		t.Fatal(err)
	}
	if _, err := v.CheckSettings(); err == nil {
		t.Error("missing external binary should fail")
	}
	dir := t.TempDir()
	bin, model := filepath.Join(dir, "whisper-cli"+exeSuffix()), filepath.Join(dir, "ggml-base.bin")
	os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755)
	os.WriteFile(model, []byte("x"), 0o644)
	if _, err := v.SaveSettings(VoiceSettingsInput{Engine: "local", WhisperBinary: bin, WhisperModel: model}); err != nil {
		t.Fatal(err)
	}
	if _, err := v.CheckSettings(); err != nil {
		t.Errorf("check local: %v", err)
	}
}

func TestVoiceStartWhenOff(t *testing.T) {
	v, _ := newVoiceFixture(t)
	if err := v.Start(); err == nil {
		t.Error("start without engine should fail")
	}
	if st := v.State(); st.State != VoiceIdle || st.Error == "" {
		t.Errorf("state = %+v", st)
	}
}

func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}
