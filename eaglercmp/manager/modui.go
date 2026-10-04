package manager

// modUI is the NaOHX-themed mod manager injected into the client page: a
// floating button that opens a modal to upload/remove .jar mods and restart
// the local server. All text is set via textContent (mod names are untrusted).
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
var css = ".nx-btn,.nx-modal,.nx-modal *{box-sizing:border-box;font-family:'Minecraft','Press Start 2P',Consolas,'Courier New',monospace}" +
".nx-btn{position:fixed;left:12px;top:12px;width:150px;height:30px;z-index:2147483646;border:0;padding:0 0 3px;background:url(` + btnPNG + `) 0 0/100% 100% no-repeat;image-rendering:pixelated;color:#e0e0e0;font-size:15px;text-shadow:2px 2px 0 #3f3f3f;cursor:pointer;display:none}" +
".nx-btn.show{display:block}" +
".nx-btn:hover,.nx-btn:focus{background-image:url(` + btnHiPNG + `);color:#ffffa0;outline:none}" +
".nx-modal{position:fixed;inset:0;z-index:2147483647;background:rgba(0,0,0,.7);display:none;align-items:center;justify-content:center}" +
".nx-modal.open{display:flex}" +
".nx-card{width:min(560px,92vw);max-height:86vh;overflow:auto;background:#3b2a1a;background-image:repeating-linear-gradient(45deg,rgba(0,0,0,.12) 0 8px,rgba(255,255,255,.03) 8px 16px);color:#e0e0e0;border:4px solid #000;box-shadow:inset 3px 3px 0 #6b4a2b,inset -3px -3px 0 #22160b;padding:18px}" +
".nx-card h2{margin:0 0 4px;font-size:20px;color:#fff;text-shadow:2px 2px 0 #3f3f3f;font-weight:700}" +
".nx-sub{font-size:12px;color:#aaa;margin-bottom:12px;text-shadow:1px 1px 0 #000}" +
".nx-pill{display:inline-block;padding:2px 8px;background:#000;border:2px solid #555;font-size:12px;margin-bottom:10px;text-transform:uppercase}" +
".nx-pill.ready{color:#55ff55;border-color:#55ff55}.nx-pill.starting{color:#ffff55;border-color:#ffff55}.nx-pill.error{color:#ff5555;border-color:#ff5555}" +
".nx-err{color:#ff5555;font-size:12px;margin:0 0 10px;white-space:pre-wrap;word-break:break-word}" +
".nx-drop{background:rgba(0,0,0,.45);border:2px dashed #aaa;padding:18px;text-align:center;font-size:13px;color:#ffff55;cursor:pointer;margin-bottom:12px}" +
".nx-drop.over,.nx-drop:hover{background:rgba(122,134,201,.35)}" +
".nx-row{display:flex;align-items:center;gap:8px;padding:6px 8px;margin-bottom:2px;background:rgba(0,0,0,.4);font-size:12px}" +
".nx-row span:first-child{flex:1;word-break:break-all}.nx-row span+span{color:#aaa}" +
".nx-b{background:url(` + btnPNG + `) 0 0/100% 100% no-repeat;image-rendering:pixelated;color:#e0e0e0;border:0;min-width:110px;height:30px;padding:0 12px 3px;font-size:14px;text-shadow:2px 2px 0 #3f3f3f;cursor:pointer}" +
".nx-b:hover{background-image:url(` + btnHiPNG + `);color:#ffffa0}.nx-b.pri{color:#ffff55}" +
".nx-foot{display:flex;gap:8px;justify-content:flex-end;margin-top:14px}.nx-msg{font-size:12px;color:#55ff55;min-height:16px;margin-top:8px}";
function el(t, c, x) { var e = document.createElement(t); if (c) e.className = c; if (x != null) e.textContent = x; return e; }
function init() {
	var st = document.createElement("style"); st.textContent = css; document.head.appendChild(st);
	var btn = el("button", "nx-btn", "Mods"); btn.type = "button";
	var modal = el("div", "nx-modal"), card = el("div", "nx-card"); modal.appendChild(card);
	document.body.appendChild(btn); document.body.appendChild(modal);
	// Only show the button on the title screen: hide it while the mouse is captured (in-game).
	setInterval(function () { btn.classList.toggle("show", performance.now() - window.__nxTitleAt < 1000 && !document.pointerLockElement); }, 250);
	var input = el("input"); input.type = "file"; input.accept = ".jar"; input.multiple = true; input.style.display = "none";
	function api(url, opt) {
		opt = opt || {}; opt.headers = H;
		return fetch(url, opt).then(function (r) { return r.json().then(function (j) { if (!r.ok) throw new Error(j.error || r.statusText); return j; }); });
	}
	function render(data, msg) {
		card.textContent = "";
		card.appendChild(el("h2", null, "Mods"));
		card.appendChild(el("div", "nx-sub", "Sodium HX Graphics - local Fabric server mods (.jar)"));
		var b = data.backend || {};
		var g = data.gateway || {};
		card.appendChild(el("span", "nx-pill " + b.state, "fabric: " + b.state));
		card.appendChild(document.createTextNode(" "));
		card.appendChild(el("span", "nx-pill " + g.state, "gateway: " + g.state));
		if (b.error) card.appendChild(el("p", "nx-err", "Fabric: " + b.error));
		if (g.error) card.appendChild(el("p", "nx-err", "Gateway: " + g.error));
		var drop = el("div", "nx-drop", "Drop .jar files here or click to upload");
		drop.onclick = function () { input.click(); };
		drop.ondragover = function (e) { e.preventDefault(); drop.classList.add("over"); };
		drop.ondragleave = function () { drop.classList.remove("over"); };
		drop.ondrop = function (e) { e.preventDefault(); upload(e.dataTransfer.files); };
		card.appendChild(drop);
		(data.mods || []).forEach(function (m) {
			var row = el("div", "nx-row"); row.appendChild(el("span", null, m.name));
			row.appendChild(el("span", null, (m.size / 1048576).toFixed(2) + " MB"));
			var rm = el("button", "nx-b", "Remove"); rm.type = "button";
			rm.onclick = function () { api(API + "?name=" + encodeURIComponent(m.name), { method: "DELETE" }).then(refresh.bind(null, "Removed " + m.name), fail); };
			row.appendChild(rm); card.appendChild(row);
		});
		if (!(data.mods || []).length) card.appendChild(el("div", "nx-sub", "No mods installed."));
		card.appendChild(el("div", "nx-msg", msg || ""));
		var foot = el("div", "nx-foot");
		var rs = el("button", "nx-b pri", "Restart server"); rs.type = "button";
		rs.onclick = function () { api(RESTART, { method: "POST" }).then(function () { refresh("Restarting - mods load on boot."); }, fail); };
		var cl = el("button", "nx-b", "Close"); cl.type = "button"; cl.onclick = function () { modal.classList.remove("open"); };
		foot.appendChild(rs); foot.appendChild(cl); card.appendChild(foot);
	}
	function fail(e) { refresh("", "Error: " + e.message); }
	function refresh(msg, err) {
		api(API).then(function (d) { render(d, err || msg); if (err) card.querySelector(".nx-msg").style.color = "#ff5555"; },
			function (e) { card.textContent = "Mod API unavailable: " + e.message; });
	}
	function upload(files) {
		var list = Array.prototype.slice.call(files), chain = Promise.resolve();
		list.forEach(function (f) {
			chain = chain.then(function () { return api(API + "?name=" + encodeURIComponent(f.name), { method: "POST", body: f }); });
		});
		chain.then(function () { refresh("Uploaded " + list.length + " mod(s). Restart the server to load them."); }, fail);
	}
	input.onchange = function () { upload(input.files); input.value = ""; };
	btn.onclick = function () { modal.classList.add("open"); refresh(""); };
	modal.onclick = function (e) { if (e.target === modal) modal.classList.remove("open"); };
	document.addEventListener("keydown", function (e) { if (e.key === "Escape") modal.classList.remove("open"); });
}
if (document.body) init(); else document.addEventListener("DOMContentLoaded", init);
})();
`
