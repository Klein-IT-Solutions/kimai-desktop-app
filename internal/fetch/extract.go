package fetch

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// Extract unpacks a .tar.gz or .zip archive into dest.
func Extract(archive, dest string) error {
	if strings.HasSuffix(archive, ".zip") {
		return ExtractZip(archive, dest)
	}
	return ExtractTarGz(archive, dest)
}

// FindFile returns the first regular file named name below dir ("" if none).
func FindFile(dir, name string) string {
	var found string
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && d.Name() == name && found == "" {
			found = p
		}
		return nil
	})
	return found
}

// safeJoin rejects archive entries that would escape dest ("zip slip").
func safeJoin(dest, name string) (string, error) {
	clean := path.Clean("/" + strings.ReplaceAll(name, "\\", "/"))
	p := filepath.Join(dest, filepath.FromSlash(clean))
	if !strings.HasPrefix(p, filepath.Clean(dest)+string(os.PathSeparator)) {
		return "", fmt.Errorf("ungültiger Pfad im Archiv: %s", name)
	}
	return p, nil
}

// ExtractTarGz unpacks a .tar.gz archive; paths are clamped into dest and only
// relative symlinks within the same directory are allowed.
func ExtractTarGz(archive, dest string) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		p, err := safeJoin(dest, h.Name)
		if err != nil {
			return err
		}
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(p, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := writeFile(p, tr, os.FileMode(h.Mode)&0o755|0o600); err != nil {
				return err
			}
		case tar.TypeSymlink:
			// Shared-library symlinks (libfoo.so -> libfoo.so.0) must stay inside dest.
			if strings.Contains(h.Linkname, "/") || strings.Contains(h.Linkname, "..") {
				return fmt.Errorf("unerlaubter Link im Archiv: %s", h.Name)
			}
			_ = os.MkdirAll(filepath.Dir(p), 0o755)
			if err := os.Symlink(h.Linkname, p); err != nil {
				return err
			}
		}
	}
}

// ExtractZip unpacks a .zip archive with paths clamped into dest.
func ExtractZip(archive, dest string) error {
	zr, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer zr.Close()
	for _, f := range zr.File {
		p, err := safeJoin(dest, f.Name)
		if err != nil {
			return err
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(p, 0o755); err != nil {
				return err
			}
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		err = writeFile(p, rc, 0o755)
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func writeFile(p string, r io.Reader, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, r); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
