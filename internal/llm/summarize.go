package llm

import (
	"context"
	"errors"
	"regexp"
	"strings"
)

// systemPrompt keeps small models on task: short bullets, no invention.
const systemPrompt = `Du formulierst Notizen zu erledigter Arbeit für eine Zeiterfassung als Stichpunkte.
Regeln:
- Antworte nur mit 1 bis 5 Stichpunkten, jeder in einer eigenen Zeile, beginnend mit "- ".
- Kurz und sachlich, im Stil "Angebot für Kunde X erstellt".
- Verwende nur Informationen aus der Notiz, erfinde nichts.
- Keine Einleitung, keine Überschrift, keine Erklärung.
- Antworte in der Sprache der Notiz.`

// MaxInput limits the text sent to the model (roughly a few minutes of speech).
const MaxInput = 4000

// ErrEmpty is returned when there is nothing to summarise.
var ErrEmpty = errors.New("Kein Text zum Zusammenfassen.")

var bulletPrefix = regexp.MustCompile(`^\s*(?:[-–—•*·]|\d+[.)])\s+`)

// Summarize turns text into bullet points using the given runtime.
func Summarize(ctx context.Context, s *Server, binary, model, text string) (string, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", ErrEmpty
	}
	if len([]rune(text)) > MaxInput {
		text = string([]rune(text)[:MaxInput])
	}
	out, err := s.Chat(ctx, binary, model, systemPrompt, "Notiz:\n"+text, 300)
	if err != nil {
		return "", err
	}
	bullets := ParseBullets(out)
	if bullets == "" {
		return "", errors.New("Das KI-Modell hat keine Stichpunkte geliefert.")
	}
	return bullets, nil
}

// ParseBullets normalises a model answer to "- " bullet lines (max. 6).
// Lines without a bullet marker are kept as bullets unless they look like
// an introduction ("Hier sind …:").
func ParseBullets(answer string) string {
	var out []string
	for _, line := range strings.Split(answer, "\n") {
		line = strings.TrimSpace(strings.ReplaceAll(line, "**", ""))
		if line == "" {
			continue
		}
		if bulletPrefix.MatchString(line) {
			line = bulletPrefix.ReplaceAllString(line, "")
		} else if strings.HasSuffix(line, ":") {
			continue // heading or introduction
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, "- "+line)
		if len(out) == 6 {
			break
		}
	}
	return strings.Join(out, "\n")
}
