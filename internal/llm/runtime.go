package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/klein-it-solutions/kimai-desktop-app/internal/fetch"
)

// Official llama.cpp release sources. llama.cpp publishes its builds as
// pre-releases, so /releases/latest (which skips pre-releases) must not be
// used: it points to an ancient release with different file names.
const (
	ReleaseBaseURL = "https://github.com/ggml-org/llama.cpp/releases"
	ReleasesAPIURL = "https://api.github.com/repos/ggml-org/llama.cpp/releases?per_page=20"
	ReleasesFeed   = "https://github.com/ggml-org/llama.cpp/releases.atom"
)

// FallbackTag is used for mirrors without release metadata.
const FallbackTag = "b11586"

// serverName is the llama.cpp server binary.
var serverName = func() string {
	if runtime.GOOS == "windows" {
		return "llama-server.exe"
	}
	return "llama-server"
}()

// platformKey is the platform part of llama.cpp's CPU build names,
// e.g. "ubuntu-x64" in llama-b1234-bin-ubuntu-x64.tar.gz.
func platformKey(goos, goarch string) (string, error) {
	arch := map[string]string{"amd64": "x64", "arm64": "arm64"}[goarch]
	if arch == "" {
		return "", fmt.Errorf("keine llama.cpp-Builds für %s/%s", goos, goarch)
	}
	switch goos {
	case "linux":
		return "ubuntu-" + arch, nil
	case "darwin":
		return "macos-" + arch, nil
	case "windows":
		return "win-cpu-" + arch, nil
	}
	return "", fmt.Errorf("keine llama.cpp-Builds für %s", goos)
}

// AssetName returns the release archive name for a tag (current naming in
// llama.cpp's release-publish.yml).
func AssetName(tag, goos, goarch string) (string, error) {
	key, err := platformKey(goos, goarch)
	if err != nil {
		return "", err
	}
	ext := ".tar.gz"
	if goos == "windows" {
		ext = ".zip"
	}
	return "llama-" + tag + "-bin-" + key + ext, nil
}

// Asset is a downloadable release archive.
type Asset struct {
	Tag  string
	Name string
	URL  string
}

type ghRelease struct {
	TagName string `json:"tag_name"`
	Draft   bool   `json:"draft"`
	Assets  []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

// ResolveFromAPI picks the newest release that actually contains a CPU
// build for the platform (works for .tar.gz and the older .zip naming).
func ResolveFromAPI(ctx context.Context, client *http.Client, apiURL, goos, goarch string) (Asset, error) {
	key, err := platformKey(goos, goarch)
	if err != nil {
		return Asset{}, err
	}
	want := regexp.MustCompile(`^llama-(b\d+)-bin-` + regexp.QuoteMeta(key) + `\.(tar\.gz|zip)$`)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return Asset{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "kimai-tracker")
	resp, err := client.Do(req)
	if err != nil {
		return Asset{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Asset{}, fmt.Errorf("GitHub-API: HTTP %d", resp.StatusCode)
	}
	var releases []ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return Asset{}, err
	}
	for _, r := range releases {
		if r.Draft {
			continue
		}
		for _, a := range r.Assets {
			if want.MatchString(a.Name) {
				return Asset{Tag: r.TagName, Name: a.Name, URL: a.URL}, nil
			}
		}
	}
	return Asset{}, errors.New("kein passendes llama.cpp-Paket in den letzten Releases")
}

var feedTag = regexp.MustCompile(`/releases/tag/(b\d+)`)

// ResolveFromFeed reads release tags from the Atom feed (no API limits) and
// returns the first whose archive exists.
func ResolveFromFeed(ctx context.Context, client *http.Client, feedURL, base, goos, goarch string) (Asset, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
	if err != nil {
		return Asset{}, err
	}
	req.Header.Set("User-Agent", "kimai-tracker")
	resp, err := client.Do(req)
	if err != nil {
		return Asset{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Asset{}, fmt.Errorf("Release-Feed: HTTP %d", resp.StatusCode)
	}
	buf := make([]byte, 512<<10)
	n, _ := ioReadFull(resp.Body, buf)
	seen := map[string]bool{}
	for _, m := range feedTag.FindAllStringSubmatch(string(buf[:n]), -1) {
		tag := m[1]
		if seen[tag] {
			continue
		}
		seen[tag] = true
		name, err := AssetName(tag, goos, goarch)
		if err != nil {
			return Asset{}, err
		}
		url := base + "/download/" + tag + "/" + name
		if exists(ctx, client, url) {
			return Asset{Tag: tag, Name: name, URL: url}, nil
		}
		if len(seen) >= 8 {
			break
		}
	}
	return Asset{}, errors.New("kein passendes llama.cpp-Paket im Release-Feed")
}

func exists(ctx context.Context, client *http.Client, url string) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return false
	}
	req.Header.Set("User-Agent", "kimai-tracker")
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// Source describes where llama.cpp is downloaded from.
type Source struct {
	APIURL  string // GitHub releases API (official source only)
	FeedURL string // Atom feed fallback (official source only)
	Base    string // .../releases base for download URLs
}

// OfficialSource is llama.cpp's GitHub repository.
var OfficialSource = Source{APIURL: ReleasesAPIURL, FeedURL: ReleasesFeed, Base: ReleaseBaseURL}

// MirrorSource serves base/download/<FallbackTag>/<asset>.
func MirrorSource(base string) Source { return Source{Base: strings.TrimRight(base, "/")} }

// Resolve finds the archive to download for this platform.
func Resolve(ctx context.Context, client *http.Client, src Source, goos, goarch string) (Asset, error) {
	var errs []error
	if src.APIURL != "" {
		a, err := ResolveFromAPI(ctx, client, src.APIURL, goos, goarch)
		if err == nil {
			return a, nil
		}
		errs = append(errs, err)
	}
	if src.FeedURL != "" {
		a, err := ResolveFromFeed(ctx, client, src.FeedURL, src.Base, goos, goarch)
		if err == nil {
			return a, nil
		}
		errs = append(errs, err)
	}
	if src.APIURL == "" && src.FeedURL == "" {
		name, err := AssetName(FallbackTag, goos, goarch)
		if err != nil {
			return Asset{}, err
		}
		return Asset{Tag: FallbackTag, Name: name, URL: src.Base + "/download/" + FallbackTag + "/" + name}, nil
	}
	return Asset{}, fmt.Errorf("llama.cpp-Version nicht ermittelbar: %w", errors.Join(errs...))
}

// RuntimeDir returns where llama.cpp builds are installed.
func RuntimeDir() (string, error) {
	dir, err := DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "llama.cpp"), nil
}

// FindServer looks for llama-server: an installed build in the data
// directory first, then PATH. It returns "" if none is found.
func FindServer() string {
	if dir, err := RuntimeDir(); err == nil {
		if found := fetch.FindFile(dir, serverName); found != "" {
			return found
		}
	}
	if p, err := exec.LookPath(serverName); err == nil {
		return p
	}
	return ""
}

// Install downloads and unpacks the llama.cpp build for this platform and
// returns the path to llama-server.
func Install(ctx context.Context, client *http.Client, src Source, progress func(done, total int64)) (string, error) {
	if client == nil {
		client = &http.Client{Timeout: 0}
	}
	resolveCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	asset, err := Resolve(resolveCtx, client, src, runtime.GOOS, runtime.GOARCH)
	cancel()
	if err != nil {
		return "", err
	}
	dir, err := RuntimeDir()
	if err != nil {
		return "", err
	}
	archive := filepath.Join(dir, asset.Name)
	magic := fetch.MagicGzip
	if strings.HasSuffix(asset.Name, ".zip") {
		magic = fetch.MagicZip
	}
	if err := fetch.File(ctx, client, asset.URL, archive, magic, progress); err != nil {
		return "", fmt.Errorf("%v (%s)", err, asset.Name)
	}
	defer os.Remove(archive)

	dest := filepath.Join(dir, asset.Tag)
	_ = os.RemoveAll(dest)
	if err := fetch.Extract(archive, dest); err != nil {
		_ = os.RemoveAll(dest)
		return "", fmt.Errorf("llama.cpp konnte nicht entpackt werden: %v", err)
	}
	// Remove older installed builds.
	if entries, err := os.ReadDir(dir); err == nil {
		for _, e := range entries {
			if e.IsDir() && e.Name() != asset.Tag {
				_ = os.RemoveAll(filepath.Join(dir, e.Name()))
			}
		}
	}
	server := fetch.FindFile(dest, serverName)
	if server == "" {
		return "", errors.New("llama-server fehlt im heruntergeladenen Paket")
	}
	return server, nil
}

func ioReadFull(r interface{ Read([]byte) (int, error) }, buf []byte) (int, error) {
	n := 0
	for n < len(buf) {
		m, err := r.Read(buf[n:])
		n += m
		if err != nil {
			return n, err
		}
	}
	return n, nil
}
