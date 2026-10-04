package manager

import _ "embed"

// Startup intro: the logo animation and its audio, merged into one MP4 so
// picture and sound are always in sync. It plays over the client while it boots.
//
//go:embed assets/intro.mp4
var introMP4 []byte

const introMP4Path = "/__eaglercmp/intro.mp4"

// introUI is a full-screen overlay with a single <video>. Browsers block
// autoplay with sound, so a click-to-start prompt is shown if play() is refused.
const introUI = `
;(function () {
"use strict";
var root = document.createElement("div");
root.style.cssText = "position:fixed;inset:0;z-index:2147483647;background:#f0303f;transition:opacity .5s;font:14px monospace;color:#fff;cursor:pointer";
var v = document.createElement("video");
v.src = "` + introMP4Path + `";
v.playsInline = true;
v.preload = "auto";
v.style.cssText = "width:100%;height:100%;object-fit:cover;display:block";
var prompt = document.createElement("div");
prompt.textContent = "CLICK TO START";
prompt.style.cssText = "position:absolute;inset:0;display:none;align-items:center;justify-content:center;letter-spacing:.2em;background:#f0303f";
root.appendChild(v); root.appendChild(prompt);
var done = false;
function finish() {
	if (done) return; done = true;
	v.pause(); root.style.opacity = "0";
	setTimeout(function () { root.remove(); }, 550);
}
v.addEventListener("ended", finish);
v.addEventListener("error", finish);
// Safety net: never leave the overlay up if playback stalls (hidden tab, decode hang).
var guard = setTimeout(finish, 12000);
v.addEventListener("loadedmetadata", function () {
	if (isFinite(v.duration)) { clearTimeout(guard); guard = setTimeout(finish, v.duration * 1000 + 3000); }
});
document.addEventListener("keydown", function (e) { if (e.key === "Escape") finish(); });
function mount() {
	document.body.appendChild(root);
	var p = v.play();
	if (p && p.catch) p.catch(function () {
		clearTimeout(guard); prompt.style.display = "flex";
		root.addEventListener("click", function () {
			prompt.style.display = "none";
			clearTimeout(guard); guard = setTimeout(finish, 12000);
			v.currentTime = 0;
			v.play().catch(finish);
		}, { once: true });
	});
}
if (document.body) mount(); else document.addEventListener("DOMContentLoaded", mount);
})();
`
