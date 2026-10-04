package manager

// modUI is the NaOHX-themed mod manager injected into the client page: a
// floating button that opens a modal to upload/remove .jar mods and restart
// the local server. All text is drawn to canvases with the game's bitmap font (mod names are untrusted).
const modUI = `
// Title-screen detector: the game UI is drawn on a WebGL canvas (no DOM), so we
// look at its pixels. The title screen is the one with 3+ wide stacked stone
// buttons; the Edit Profile screen only has one wide button. The canvas is
// forced to keep its drawing buffer so it can be sampled.
;(function () {
window.__nxTitleAt = 0;
var gc = HTMLCanvasElement.prototype.getContext;
HTMLCanvasElement.prototype.getContext = function (t, o) {
	if (/webgl/.test(t)) o = Object.assign({}, o, { preserveDrawingBuffer: true });
	return gc.call(this, t, o);
};
var probe = document.createElement("canvas"), px = probe.getContext("2d", { willReadFrequently: true });
function bands(src) {
	var w = Math.max(1, Math.round(src.width / 4)), h = Math.max(1, Math.round(src.height / 4));
	probe.width = w; probe.height = h; px.imageSmoothingEnabled = false;
	px.drawImage(src, 0, 0, w, h);
	var d = px.getImageData(0, 0, w, h).data, need = w * 0.2, n = 0, inBand = false;
	for (var y = 0; y < h; y++) {
		var run = 0, best = 0;
		for (var x = 0; x < w; x++) {
			var i = (y * w + x) * 4, r = d[i], g = d[i + 1], b = d[i + 2];
			if (r > 60 && r < 160 && Math.abs(r - g) < 10 && Math.abs(g - b) < 14) { if (++run > best) best = run; } else run = 0;
		}
		if (best >= need) { if (!inBand) { n++; inBand = true; } } else inBand = false;
	}
	return n;
}
setInterval(function () {
	if (document.hidden) return;
	var c = document.querySelector("canvas");
	if (!c || !c.width) return;
	try { var n = bands(c); window.__nxBands = n; if (n >= 3) window.__nxTitleAt = performance.now(); } catch (e) { window.__nxErr = String(e); }
}, 300);
})();
;(function () {
"use strict";
var API = "` + modsPath + `", RESTART = "` + backendRestartPath + `", H = { "X-EaglerCMP": "1" };
var BTN = "url(` + btnPNG + `) 3 fill / 6px / 0 stretch", BTNHI = "url(` + btnHiPNG + `) 3 fill / 6px / 0 stretch";
var css = ".nx-btn,.nx-modal,.nx-modal *{box-sizing:border-box;image-rendering:pixelated}" +
".nx-btn,.nx-b{display:flex;align-items:center;justify-content:center;border:6px solid #000;border-image:" + BTN + ";background:none;padding:0;cursor:pointer;height:30px;outline:none}" +
".nx-btn{position:fixed;left:12px;top:12px;width:150px;z-index:2147483646;display:none}" +
".nx-btn.show{display:flex}" +
".nx-btn:hover,.nx-btn:focus,.nx-b:hover,.nx-b:focus{border-image:" + BTNHI + "}" +
".nx-modal{position:fixed;inset:0;z-index:2147483647;background:linear-gradient(rgba(0,0,0,.6),rgba(0,0,0,.6)),url(` + dirtPNG + `) 0 0/64px;display:none;align-items:center;justify-content:center}" +
".nx-modal.open{display:flex}" +
".nx-card{width:min(600px,94vw);max-height:88vh;overflow:auto;border:12px solid transparent;border-image:url(` + popupPNG + `) 6 fill / 12px / 0 stretch;padding:8px}" +
".nx-card canvas,.nx-btn canvas,.nx-b canvas{display:block;image-rendering:pixelated;max-width:100%}" +
".nx-card h2{margin:0 0 6px}.nx-sub{margin-bottom:12px}" +
".nx-pill{display:inline-block;padding:2px 6px;background:#000;border:2px solid #555;margin:0 6px 10px 0}" +
".nx-pill.ready{border-color:#55ff55}.nx-pill.starting{border-color:#ffff55}.nx-pill.error{border-color:#ff5555}" +
".nx-err{margin:0 0 10px}" +
".nx-drop{background:rgba(0,0,0,.5);border:2px dashed #aaa;padding:16px;display:flex;justify-content:center;cursor:pointer;margin-bottom:12px}" +
".nx-drop.over,.nx-drop:hover{background:rgba(122,134,201,.35)}" +
".nx-row{display:flex;align-items:center;gap:10px;padding:6px 8px;margin-bottom:2px;background:rgba(0,0,0,.45)}" +
".nx-row span:first-child{flex:1;min-width:0}" +
".nx-b{min-width:120px;padding:0 6px}" +
".nx-foot{display:flex;gap:8px;justify-content:flex-end;margin-top:14px}.nx-msg{min-height:18px;margin-top:8px}";
// Minecraft bitmap font (textures/font/ascii.png: 16x16 grid of 8x8 glyphs, proportional width).
var FONT = new Image(), GW = [], fontReady = false, waiting = [];
FONT.onload = function () {
	var c = document.createElement("canvas"); c.width = c.height = 128;
	var x = c.getContext("2d"); x.drawImage(FONT, 0, 0);
	var d = x.getImageData(0, 0, 128, 128).data;
	for (var k = 0; k < 256; k++) {
		var w = 0, cx = (k % 16) * 8, cy = (k >> 4) * 8;
		for (var col = 0; col < 8; col++) for (var row = 0; row < 8; row++) if (d[((cy + row) * 128 + cx + col) * 4 + 3] > 0) w = col + 1;
		GW[k] = k === 32 ? 3 : w;
	}
	fontReady = true; waiting.forEach(function (f) { f(); });
};
FONT.src = "` + fontPNG + `";
function code(ch) { var k = ch.charCodeAt(0); return k < 256 ? k : 63; }
function adv(ch) { return GW[code(ch)] + 1; }
function wrap(text, max) {
	var lines = [], line = "", w = 0;
	String(text).split("\n").forEach(function (para) {
		para.split(" ").forEach(function (word, wi) {
			var ww = 0, i; for (i = 0; i < word.length; i++) ww += adv(word[i]);
			var sp = line && wi ? 4 : 0;
			if (line && max && w + sp + ww > max) { lines.push(line); line = ""; w = 0; sp = 0; }
			if (sp) { line += " "; w += sp; }
			for (i = 0; i < word.length; i++) {
				var a = adv(word[i]);
				if (max && w + a > max && line) { lines.push(line); line = ""; w = 0; }
				line += word[i]; w += a;
			}
		});
		lines.push(line); line = ""; w = 0;
	});
	return lines;
}
function shade(hex) { var n = parseInt(hex.slice(1), 16); return "rgb(" + ((n >> 16) >> 2) + "," + (((n >> 8) & 255) >> 2) + "," + ((n & 255) >> 2) + ")"; }
// T renders text with the game's own font into a canvas.
function T(text, color, scale, maxW) {
	var c = document.createElement("canvas"); scale = scale || 2; color = color || "#ffffff";
	c.setAttribute("role", "img"); c.setAttribute("aria-label", String(text));
	function draw() {
		var lines = wrap(text, maxW ? maxW / scale : 0), wpx = 0;
		lines.forEach(function (l) { var w = 0; for (var i = 0; i < l.length; i++) w += adv(l[i]); if (w > wpx) wpx = w; });
		c.width = Math.max(1, wpx * scale + scale); c.height = lines.length * 9 * scale + scale;
		var m = document.createElement("canvas"); m.width = c.width; m.height = c.height;
		var mx = m.getContext("2d"); mx.imageSmoothingEnabled = false;
		lines.forEach(function (l, li) {
			var x = 0;
			for (var i = 0; i < l.length; i++) {
				var k = code(l[i]);
				mx.drawImage(FONT, (k % 16) * 8, (k >> 4) * 8, 8, 8, x * scale, li * 9 * scale, 8 * scale, 8 * scale);
				x += GW[k] + 1;
			}
		});
		function tint(col) {
			var t = document.createElement("canvas"); t.width = m.width; t.height = m.height;
			var tx = t.getContext("2d"); tx.drawImage(m, 0, 0); tx.globalCompositeOperation = "source-in"; tx.fillStyle = col; tx.fillRect(0, 0, t.width, t.height); return t;
		}
		var g = c.getContext("2d"); g.drawImage(tint(shade(color.length === 7 ? color : "#ffffff")), scale, scale); g.drawImage(tint(color), 0, 0);
	}
	if (fontReady) draw(); else waiting.push(draw);
	return c;
}
function el(t, c, x, color, scale, maxW) { var e = document.createElement(t); if (c) e.className = c; if (x != null) e.appendChild(T(x, color, scale, maxW)); return e; }
// mcBtn builds a game-style button; the label turns yellow on hover like the game's.
function mcBtn(cls, label, color, onclick) {
	var b = el("button", cls); b.type = "button"; color = color || "#e0e0e0";
	function set(col) { b.textContent = ""; b.appendChild(T(label, col, 2)); }
	set(color);
	b.onmouseenter = b.onfocus = function () { set("#ffffa0"); };
	b.onmouseleave = b.onblur = function () { set(color); };
	b.onclick = onclick; return b;
}
function init() {
	var st = document.createElement("style"); st.textContent = css; document.head.appendChild(st);
	var modal = el("div", "nx-modal"), card = el("div", "nx-card"); modal.appendChild(card);
	var btn = mcBtn("nx-btn", "Mods", null, function () { modal.classList.add("open"); refresh(""); });
	document.body.appendChild(btn); document.body.appendChild(modal);
	// Only show the button on the title screen: hide it while the mouse is captured (in-game).
	setInterval(function () { btn.classList.toggle("show", performance.now() - window.__nxTitleAt < 1000 && !document.pointerLockElement); }, 250);
	var input = el("input"); input.type = "file"; input.accept = ".jar"; input.multiple = true; input.style.display = "none";
	function api(url, opt) {
		opt = opt || {}; opt.headers = H;
		return fetch(url, opt).then(function (r) { return r.json().then(function (j) { if (!r.ok) throw new Error(j.error || r.statusText); return j; }); });
	}
	var PILL = { ready: "#55ff55", starting: "#ffff55", error: "#ff5555" };
	function render(data, msg, isErr) {
		card.textContent = "";
		card.appendChild(el("h2", null, "Mods", "#ffffff", 3));
		card.appendChild(el("div", "nx-sub", "Sodium HX Graphics - local Fabric server mods (.jar)", "#aaaaaa", 2, 520));
		var b = data.backend || {}, g = data.gateway || {};
		card.appendChild(el("span", "nx-pill " + b.state, "fabric: " + b.state, PILL[b.state] || "#aaaaaa"));
		card.appendChild(el("span", "nx-pill " + g.state, "gateway: " + g.state, PILL[g.state] || "#aaaaaa"));
		if (b.error) card.appendChild(el("p", "nx-err", "Fabric: " + b.error, "#ff5555", 2, 520));
		if (g.error) card.appendChild(el("p", "nx-err", "Gateway: " + g.error, "#ff5555", 2, 520));
		var drop = el("div", "nx-drop", "Drop .jar files here or click to upload", "#ffff55");
		drop.onclick = function () { input.click(); };
		drop.ondragover = function (e) { e.preventDefault(); drop.classList.add("over"); };
		drop.ondragleave = function () { drop.classList.remove("over"); };
		drop.ondrop = function (e) { e.preventDefault(); upload(e.dataTransfer.files); };
		card.appendChild(drop);
		(data.mods || []).forEach(function (m) {
			var row = el("div", "nx-row"), name = el("span"); name.appendChild(T(m.name, "#ffffff", 2, 300)); row.appendChild(name);
			row.appendChild(el("span", null, (m.size / 1048576).toFixed(2) + " MB", "#aaaaaa"));
			row.appendChild(mcBtn("nx-b", "Remove", null, function () { api(API + "?name=" + encodeURIComponent(m.name), { method: "DELETE" }).then(refresh.bind(null, "Removed " + m.name), fail); }));
			card.appendChild(row);
		});
		if (!(data.mods || []).length) card.appendChild(el("div", "nx-sub", "No mods installed.", "#aaaaaa"));
		card.appendChild(el("div", "nx-msg", msg || "", isErr ? "#ff5555" : "#55ff55", 2, 520));
		var foot = el("div", "nx-foot");
		foot.appendChild(mcBtn("nx-b", "Restart server", "#55ff55", function () { api(RESTART, { method: "POST" }).then(function () { refresh("Restarting - mods load on boot."); }, fail); }));
		foot.appendChild(mcBtn("nx-b", "Close", null, function () { modal.classList.remove("open"); }));
		card.appendChild(foot);
	}
	function fail(e) { refresh("", "Error: " + e.message); }
	function refresh(msg, err) {
		api(API).then(function (d) { render(d, err || msg, !!err); },
			function (e) { card.textContent = ""; card.appendChild(T("Mod API unavailable: " + e.message, "#ff5555", 2, 520)); });
	}
	function upload(files) {
		var list = Array.prototype.slice.call(files), chain = Promise.resolve();
		list.forEach(function (f) {
			chain = chain.then(function () { return api(API + "?name=" + encodeURIComponent(f.name), { method: "POST", body: f }); });
		});
		chain.then(function () { refresh("Uploaded " + list.length + " mod(s). Restart the server to load them."); }, fail);
	}
	input.onchange = function () { upload(input.files); input.value = ""; };
	modal.onclick = function (e) { if (e.target === modal) modal.classList.remove("open"); };
	document.addEventListener("keydown", function (e) { if (e.key === "Escape") modal.classList.remove("open"); });
}
if (document.body) init(); else document.addEventListener("DOMContentLoaded", init);
})();
`
