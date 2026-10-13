package app

import (
	"context"
	"errors"

	"github.com/klein-it-solutions/kimai-desktop-app/internal/fetch"
)

func downloadGGUF(ctx context.Context, url, dest string, progress func(int64, int64)) error {
	err := fetch.File(ctx, nil, url, dest, fetch.MagicGGUF, progress)
	if errors.Is(err, fetch.ErrWrongType) {
		return errors.New("Die heruntergeladene Datei ist kein GGUF-Modell")
	}
	return err
}
