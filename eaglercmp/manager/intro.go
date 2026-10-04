package manager

import _ "embed"

// Startup intro: the logo animation (3.87s GIF) and its audio (~4.36s MP3)
// play together over the client while it boots.
var (
	//go:embed assets/intro.gif
	introGIF []byte
	//go:embed assets/intro.mp3
	introMP3 []byte
)

const (
	introGIFPath = "/__eaglercmp/intro.gif"
	introMP3Path = "/__eaglercmp/intro.mp3"
)

// introUI is a full-screen overlay. The GIF is restarted (cache-busted) at the
// exact moment the audio starts so they stay in sync; when the GIF ends its
// last frame is frozen on a canvas until the audio finishes, then it fades out.
// Browsers block autoplay audio, so a click-to-start prompt is shown if needed.
const introUI = `
;(function () {
"use strict";
if (sessionStorage.getItem("nxIntroDone")) return;
var GIF_MS = 3870, BG = "#f0303f";
var root = document.createElement("div");
root.style.cssText = "position:fixed;inset:0;z-index:2147483647;background:" + BG + ";display:flex;align-items:center;justify-content:center;transition:opacity .5s;font:14px monospace;color:#fff;cursor:pointer";
var img = document.createElement("img");
img.style.cssText = "max-width:100%;max-height:100%;object-fit:contain;display:none";
var canvas = document.createElement("canvas");
canvas.style.cssText = img.style.cssText;
var prompt = document.createElement("div");
prompt.textContent = "CLICK TO START";
prompt.style.cssText = "letter-spacing:.2em;display:none";
root.appendChild(img); root.appendChild(canvas); root.appendChild(prompt);
var audio = new Audio("` + introMP3Path + `");
audio.preload = "auto";
var finished = false, freezeTimer;
function finish() {
	if (finished) return; finished = true;
	clearTimeout(freezeTimer); audio.pause();
	sessionStorage.setItem("nxIntroDone", "1");
	root.style.opacity = "0";
	setTimeout(function () { root.remove(); }, 550);
}
function freeze() {
	canvas.width = img.naturalWidth; canvas.height = img.naturalHeight;
	try { canvas.getContext("2d").drawImage(img, 0, 0); canvas.style.display = ""; img.style.display = "none"; } catch (e) {}
}
function start() {
	prompt.style.display = "none";
	audio.currentTime = 0;
	var p = audio.play();
	return p && p.then ? p.then(go) : (go(), 0);
}
function go() {
	canvas.style.display = "none";
	img.style.display = "";
	img.src = "` + introGIFPath + `?t=" + Date.now(); // restart the GIF in step with the audio
	freezeTimer = setTimeout(freeze, GIF_MS - 40);
}
audio.addEventListener("ended", finish);
root.addEventListener("click", function () { if (started) finish(); });
document.addEventListener("keydown", function (e) { if (started && (e.key === "Escape" || e.key === " ")) finish(); });
var started = false;
function mount() {
	document.body.appendChild(root);
	start().then(function () { started = true; }, function () {
		prompt.style.display = "";
		root.addEventListener("click", function once() {
			root.removeEventListener("click", once);
			start().then(function () { started = true; }, finish);
		});
	});
}
if (document.body) mount(); else document.addEventListener("DOMContentLoaded", mount);
})();
`
