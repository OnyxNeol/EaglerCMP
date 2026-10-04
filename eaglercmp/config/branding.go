package config

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// "Sodium HX Graphics" (short: NaOHX) is a software brand name for the
// EaglerCMP performance profile: the set of browser-engine flags (GPU
// rasterisation, no background throttling, zero-copy uploads) and the window
// theme applied to the Eaglercraft client. It is a marketing label for a
// software graphics pipeline only and does not refer to any chemical.
//
// The name nods to Sodium by CaffeineMC (https://github.com/CaffeineMC), the
// Java Edition rendering mod. EaglerCMP contains no CaffeineMC code.

// Branding tokens used by the CLI, logs, the client window and naohx.json.
const (
	ProductName   = "EaglerCMP"
	ProductLong   = "EaglerCraft with Mods and Performance"
	EngineName    = "Sodium HX Graphics"
	EngineShort   = "NaOHX"
	EngineTagline = "GPU-first performance profile for the Eaglercraft web runtime"
)

// LauncherVersion is overridden at release time with
// -ldflags "-X github.com/OnyxNeol/eaglercmp/config.LauncherVersion=<tag>".
var LauncherVersion = "0.2.0-dev"

// WindowTitle is the title shown on the client window.
func WindowTitle() string {
	return fmt.Sprintf("%s - %s (%s)", ProductName, EngineName, EngineShort)
}

// Credit is one attribution entry shown in the splash and written to disk.
type Credit struct {
	Component string `json:"component"`
	Author    string `json:"author"`
	License   string `json:"license"`
	URL       string `json:"url"`
}

// Credits lists the upstream projects EaglerCMP builds on or references.
var Credits = []Credit{
	{"Eaglercraft", "lax1dude & Eaglercraft contributors", "see upstream", "https://github.com/eymenwsmc/playopspt.github.io"},
	{"TeaVM (Wasm-GC compiler)", "Alexey Andreev & contributors", "Apache-2.0", "https://github.com/konsoletyper/teavm"},
	{"Sodium (name inspiration)", "CaffeineMC", "Polyform Shield 1.0.0", "https://github.com/CaffeineMC/sodium"},
}

// Splash writes the startup banner with attribution.
func Splash(w io.Writer) {
	line := strings.Repeat("=", 64)
	fmt.Fprintln(w, line)
	fmt.Fprintf(w, " %s v%s - %s\n", ProductName, LauncherVersion, ProductLong)
	fmt.Fprintf(w, " Performance engine: %s (%s)\n", EngineName, EngineShort)
	fmt.Fprintf(w, "   %s\n", EngineTagline)
	fmt.Fprintln(w, "   Eaglercraft by lax1dude & contributors | 'Sodium' name credit: CaffeineMC")
	fmt.Fprintln(w, line)
}

// PrintCredits writes the full attribution table.
func PrintCredits(w io.Writer) {
	fmt.Fprintf(w, "%s credits (%s is a software brand for the launcher's performance profile):\n", ProductName, EngineShort)
	for _, c := range Credits {
		fmt.Fprintf(w, "  - %-26s %s [%s] %s\n", c.Component, c.Author, c.License, c.URL)
	}
}

// BrandingFile is written to the instance root and served to the client page.
type BrandingFile struct {
	Product         string   `json:"product"`
	ProductLong     string   `json:"productLong"`
	LauncherVersion string   `json:"launcherVersion"`
	Engine          string   `json:"engine"`
	EngineShort     string   `json:"engineShort"`
	Description     string   `json:"description"`
	Performance     bool     `json:"performance"`
	Flags           []string `json:"flags"`
	Credits         []Credit `json:"credits"`
}

// Branding returns the branding document for the given profile flags.
func Branding(performance bool, flags []string) BrandingFile {
	if flags == nil {
		flags = []string{}
	}
	return BrandingFile{
		Product:         ProductName,
		ProductLong:     ProductLong,
		LauncherVersion: LauncherVersion,
		Engine:          EngineName,
		EngineShort:     EngineShort,
		Description:     EngineName + " (" + EngineShort + ") is the software brand for EaglerCMP's GPU-first window profile for the Eaglercraft web client.",
		Performance:     performance,
		Flags:           flags,
		Credits:         Credits,
	}
}

// WriteBrandingFile writes <dir>/naohx.json.
func WriteBrandingFile(dir string, b BrandingFile) error {
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "naohx.json"), append(data, '\n'), 0o644)
}
