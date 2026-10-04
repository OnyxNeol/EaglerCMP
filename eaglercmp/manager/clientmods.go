package manager

import (
	_ "embed"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Client-side mods are .js / .wasm files in <instance>/client-mods; assets
// (textures, sounds, models, anything the page can load) live in
// <instance>/client-assets. client-mods/loader.js is embedded and injected into
// the game page head: it exposes window.eaglercmp.registerMod() and loads the
// mods in file-name order once the page has loaded.
const (
	clientModsPath   = "/__eaglercmp/client-mods"
	clientAssetsPath = "/__eaglercmp/client-assets"
	maxAssetList     = 5000
)

//go:embed client-mods/loader.js
var clientModLoader string

var (
	clientModRe   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+() -]{0,120}\.(js|wasm)$`)
	clientAssetRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+() -]{0,120}$`)
)

func hasPrefixPath(p, prefix string) bool { return p == prefix || strings.HasPrefix(p, prefix+"/") }

// serveClientMods serves the mod list (p == clientModsPath) or one mod file.
func (h *Handler) serveClientMods(w http.ResponseWriter, r *http.Request, p string) {
	w.Header().Set("Cache-Control", "no-store")
	if p == clientModsPath {
		names := []string{}
		entries, _ := os.ReadDir(h.ClientMods)
		for _, e := range entries {
			if e.Type().IsRegular() && clientModRe.MatchString(e.Name()) {
				names = append(names, e.Name())
			}
		}
		sort.Strings(names)
		writeJSON(w, 200, map[string]any{"mods": names})
		return
	}
	name := strings.TrimPrefix(p, clientModsPath+"/")
	if !clientModRe.MatchString(name) {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", contentTypes[strings.ToLower(path.Ext(name))])
	http.ServeFile(w, r, filepath.Join(h.ClientMods, name))
}

// serveClientAssets lists files (p == clientAssetsPath) or serves one.
func (h *Handler) serveClientAssets(w http.ResponseWriter, r *http.Request, p string) {
	w.Header().Set("Cache-Control", "no-store")
	if p == clientAssetsPath {
		files := []string{}
		filepath.WalkDir(h.ClientAssets, func(fp string, d fs.DirEntry, err error) error {
			if err != nil || len(files) >= maxAssetList {
				return filepath.SkipDir
			}
			if strings.HasPrefix(d.Name(), ".") && fp != h.ClientAssets {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if rel, e := filepath.Rel(h.ClientAssets, fp); e == nil && d.Type().IsRegular() && validAssetPath(filepath.ToSlash(rel)) {
				files = append(files, filepath.ToSlash(rel))
			}
			return nil
		})
		sort.Strings(files)
		writeJSON(w, 200, map[string]any{"files": files})
		return
	}
	rel := strings.TrimPrefix(p, clientAssetsPath+"/")
	if !validAssetPath(rel) {
		http.NotFound(w, r)
		return
	}
	full := filepath.Join(h.ClientAssets, filepath.FromSlash(rel))
	if st, err := os.Stat(full); err != nil || !st.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	if ct, ok := contentTypes[strings.ToLower(path.Ext(rel))]; ok {
		w.Header().Set("Content-Type", ct)
	}
	http.ServeFile(w, r, full)
}

// validAssetPath accepts only clean relative paths of safe segments (no "..", no dotfiles).
func validAssetPath(rel string) bool {
	if rel == "" || path.Clean(rel) != rel {
		return false
	}
	for _, seg := range strings.Split(rel, "/") {
		if !clientAssetRe.MatchString(seg) || seg == ".." {
			return false
		}
	}
	return true
}
