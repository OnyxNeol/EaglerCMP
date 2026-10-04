package manager

import (
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Client-side mods are plain JavaScript files in <instance>/client-mods.
// They are listed at clientModsPath and loaded into the game page by
// clientModLoader, after the client's own scripts, in file-name order.
const clientModsPath = "/__eaglercmp/client-mods"

var clientModRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+() -]{0,120}\.js$`)

// serveClientMods serves the list (path == clientModsPath) or one script.
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
	if !clientModRe.MatchString(name) || path.Base(name) != name {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", contentTypes[".js"])
	http.ServeFile(w, r, filepath.Join(h.ClientMods, name))
}

// clientModLoader runs in the game page: it injects every client mod script
// once the client has loaded, isolating failures so one bad mod can't break
// the game or the others.
const clientModLoader = `
;(function () {
"use strict";
function boot() {
	fetch("` + clientModsPath + `", { cache: "no-store" }).then(function (r) { return r.json(); }).then(function (d) {
		window.eaglercmpMods = d.mods;
		(d.mods || []).reduce(function (p, n) {
			return p.then(function () {
				return new Promise(function (ok) {
					var s = document.createElement("script");
					s.src = "` + clientModsPath + `/" + encodeURIComponent(n);
					s.onload = s.onerror = function () { console.log("[EaglerCMP] client mod loaded: " + n); ok(); };
					document.body.appendChild(s);
				});
			});
		}, Promise.resolve());
	}).catch(function (e) { console.warn("[EaglerCMP] client mods unavailable", e); });
}
if (document.readyState === "complete") boot(); else window.addEventListener("load", boot);
})();
`
