package manager

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/OnyxNeol/eaglercmp/config"
)

// SystemBrowser is the config value that opens the user's default browser
// instead of a dedicated app window.
const SystemBrowser = "system"

// NaOHXFlags is the Sodium HX Graphics performance profile: Chromium flags
// that keep WebGL on the GPU and stop the engine from throttling the game
// loop when the window is in the background or occluded.
func NaOHXFlags() []string {
	return []string{
		"--ignore-gpu-blocklist",
		"--enable-gpu-rasterization",
		"--enable-zero-copy",
		"--disable-background-timer-throttling",
		"--disable-renderer-backgrounding",
		"--disable-backgrounding-occluded-windows",
	}
}

// ProfileFlags returns the NaOHX flags if the profile is enabled.
func ProfileFlags(cfg *config.Config) []string {
	if !cfg.Performance {
		return nil
	}
	return NaOHXFlags()
}

// BrowserArgs builds the command line for a Chromium-based app window with a
// dedicated profile, so the client's saved worlds never mix with the user's
// normal browsing data.
func BrowserArgs(cfg *config.Config, profileDir, url string) []string {
	args := []string{
		"--app=" + url,
		"--user-data-dir=" + profileDir,
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-default-apps",
		"--disable-sync",
		"--disable-extensions",
		"--autoplay-policy=no-user-gesture-required",
		fmt.Sprintf("--window-size=%d,%d", cfg.Width, cfg.Height),
		"--enable-logging=stderr",
		"--v=0",
	}
	disabled := "Translate"
	if cfg.Performance {
		disabled += ",CalculateNativeWinOcclusion"
	}
	args = append(args, "--disable-features="+disabled)
	if runtime.GOOS == "linux" {
		args = append(args, "--class="+config.ProductName)
	}
	if cfg.Fullscreen {
		args = append(args, "--start-fullscreen")
	}
	args = append(args, ProfileFlags(cfg)...)
	return append(args, cfg.BrowserArgs...)
}

// browserCandidates lists well-known Chromium-based browser locations.
func browserCandidates() []string {
	switch runtime.GOOS {
	case "windows":
		var out []string
		for _, env := range []string{"ProgramFiles(x86)", "ProgramFiles", "LocalAppData"} {
			base := os.Getenv(env)
			if base == "" {
				continue
			}
			out = append(out,
				filepath.Join(base, "Microsoft", "Edge", "Application", "msedge.exe"),
				filepath.Join(base, "Google", "Chrome", "Application", "chrome.exe"),
				filepath.Join(base, "BraveSoftware", "Brave-Browser", "Application", "brave.exe"),
				filepath.Join(base, "Chromium", "Application", "chrome.exe"),
			)
		}
		return append(out, "msedge", "chrome")
	case "darwin":
		apps := []string{
			"Google Chrome.app/Contents/MacOS/Google Chrome",
			"Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
			"Chromium.app/Contents/MacOS/Chromium",
			"Brave Browser.app/Contents/MacOS/Brave Browser",
		}
		var out []string
		roots := []string{"/Applications"}
		if home, err := os.UserHomeDir(); err == nil {
			roots = append(roots, filepath.Join(home, "Applications"))
		}
		for _, r := range roots {
			for _, a := range apps {
				out = append(out, filepath.Join(r, a))
			}
		}
		return out
	}
	return []string{
		"google-chrome", "google-chrome-stable", "chromium", "chromium-browser",
		"microsoft-edge", "microsoft-edge-stable", "brave-browser",
	}
}

// FindBrowser returns the browser used for the app window: the configured
// path, $EAGLERCMP_BROWSER, or the first installed Chromium-based browser.
func FindBrowser(configured string) (string, error) {
	if configured == "" {
		configured = os.Getenv("EAGLERCMP_BROWSER")
	}
	if configured != "" {
		if p, err := exec.LookPath(configured); err == nil {
			return p, nil
		}
		return "", fmt.Errorf("configured browser %q not found", configured)
	}
	for _, c := range browserCandidates() {
		if strings.ContainsRune(c, os.PathSeparator) {
			if st, err := os.Stat(c); err == nil && !st.IsDir() {
				return c, nil
			}
			continue
		}
		if p, err := exec.LookPath(c); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("no Chromium-based browser (Chrome, Edge, Chromium, Brave) found")
}

// OpenSystemBrowser opens url in the user's default browser.
func OpenSystemBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}
