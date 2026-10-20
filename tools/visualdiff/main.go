// Command visualdiff captures the local component gallery and compares it with
// its approved PNG baseline.
package main

import (
	"errors"
	"flag"
	"fmt"
	"image"
	"image/png"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

var (
	// errChromeMissing means no usable Chrome/Chromium was found (exit 2).
	errChromeMissing = errors.New("chrome missing")
	// errOverThreshold means the screenshot differs from the baseline too much (exit 1).
	errOverThreshold = errors.New("visual diff exceeds threshold")
)

// exitCode maps a run error to a process exit code: 0 ok, 2 Chrome missing,
// 1 for everything else (diff over threshold, missing baseline, gallery down).
func exitCode(err error) int {
	switch {
	case err == nil:
		return 0
	case errors.Is(err, errChromeMissing):
		return 2
	default:
		return 1
	}
}

// cropTo clips img to its top-left w x h region; headless Chrome may return a
// screenshot taller than the requested window. A smaller image is an error.
func cropTo(img image.Image, w, h int) (image.Image, error) {
	b := img.Bounds()
	if b.Dx() < w || b.Dy() < h {
		return nil, fmt.Errorf("screenshot is %dx%d, smaller than expected %dx%d", b.Dx(), b.Dy(), w, h)
	}
	if b.Dx() == w && b.Dy() == h {
		return img, nil
	}
	sub, ok := img.(interface {
		SubImage(image.Rectangle) image.Image
	})
	if !ok {
		return nil, errors.New("screenshot cannot be cropped")
	}
	return sub.SubImage(image.Rect(b.Min.X, b.Min.Y, b.Min.X+w, b.Min.Y+h)), nil
}

func diff(baseline, actual image.Image) (float64, error) {
	a, b := baseline.Bounds(), actual.Bounds()
	if a.Dx() != b.Dx() || a.Dy() != b.Dy() {
		return 0, fmt.Errorf("image dimensions differ: baseline %dx%d, actual %dx%d", a.Dx(), a.Dy(), b.Dx(), b.Dy())
	}
	if a.Empty() {
		return 0, errors.New("empty image")
	}
	changed := 0
	for y := 0; y < a.Dy(); y++ {
		for x := 0; x < a.Dx(); x++ {
			// Color implementations may differ; compare normalized 16-bit RGBA.
			br, bg, bb, ba := baseline.At(a.Min.X+x, a.Min.Y+y).RGBA()
			ar, ag, ab, aa := actual.At(b.Min.X+x, b.Min.Y+y).RGBA()
			if br != ar || bg != ag || bb != ab || ba != aa {
				changed++
			}
		}
	}
	return float64(changed) / float64(a.Dx()*a.Dy()), nil
}

func readPNG(path string) (image.Image, error) {
	f, err := os.Open(path) // #nosec G304 -- dev tool; path is the developer's -baseline flag or its own temp dir
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return png.Decode(f)
}

func chromePath() (string, error) {
	if configured := os.Getenv("CHROME"); configured != "" {
		if _, err := os.Stat(configured); err == nil { // #nosec G703 -- dev tool; CHROME is the developer's own setting
			return configured, nil
		}
		return "", fmt.Errorf("%w: CHROME=%q does not exist", errChromeMissing, configured)
	}
	for _, name := range []string{"google-chrome", "chromium", "chromium-browser", "chrome"} {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
	}
	macChrome := "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
	if _, err := os.Stat(macChrome); err == nil {
		return macChrome, nil
	}
	return "", fmt.Errorf("%w: install Chrome/Chromium or set CHROME to its executable path", errChromeMissing)
}

func run() error {
	baselinePath := flag.String("baseline", "internal/server/web/dev/baselines/gallery.png", "approved PNG baseline")
	defaultURL := (&url.URL{Scheme: "http", Host: "127.0.0.1:8080", Path: "/dev/gallery.html"}).String()
	urlString := flag.String("url", defaultURL, "local gallery URL")
	cacheDir := flag.String("cache-dir", ".gocache", "directory for temporary screenshot and Chrome profile")
	threshold := flag.Float64("threshold", 0.01, "maximum fraction of changed pixels")
	flag.Parse()
	if *threshold < 0 || *threshold > 1 {
		return errors.New("threshold must be between 0 and 1")
	}
	chrome, err := chromePath()
	if err != nil {
		return err
	}
	baseline, err := readPNG(*baselinePath)
	if err != nil {
		return fmt.Errorf("baseline %s: %w (approved baselines are added in V1f)", *baselinePath, err)
	}
	u, err := url.Parse(*urlString)
	if err != nil || u.Scheme != "http" || u.Hostname() == "" || net.ParseIP(u.Hostname()) == nil || !net.ParseIP(u.Hostname()).IsLoopback() {
		return errors.New("gallery URL must use HTTP on a loopback IP address")
	}
	client := http.Client{Timeout: 5 * time.Second}
	response, err := client.Get(u.String())
	if err != nil {
		return fmt.Errorf("gallery unavailable at %s: %w (start the server with --dev)", u, err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("gallery returned HTTP %d at %s (start the server with --dev)", response.StatusCode, u)
	}
	if err := os.MkdirAll(*cacheDir, 0o750); err != nil {
		return err
	}
	workDir, err := os.MkdirTemp(*cacheDir, "visualdiff-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(workDir)
	screenshotPath := filepath.Join(workDir, "gallery.png")
	cmd := exec.Command(chrome, "--headless", "--disable-gpu", "--no-first-run", "--virtual-time-budget=2000", "--user-data-dir="+filepath.Join(workDir, "profile"), "--screenshot="+screenshotPath, "--window-size=1448,1086", u.String()) // #nosec G204 -- dev tool; chrome comes from CHROME or PATH lookup
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("chrome screenshot failed: %w: %s", err, output)
	}
	actual, err := readPNG(screenshotPath)
	if err != nil {
		return fmt.Errorf("chrome screenshot: %w", err)
	}
	bb := baseline.Bounds()
	if actual, err = cropTo(actual, bb.Dx(), bb.Dy()); err != nil {
		return err
	}
	changed, err := diff(baseline, actual)
	if err != nil {
		return err
	}
	fmt.Printf("visual diff: %.4f%% changed (threshold %.4f%%)\n", changed*100, *threshold*100)
	if changed > *threshold {
		return errOverThreshold
	}
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(exitCode(err))
	}
}
