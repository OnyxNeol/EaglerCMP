package manager

// modUI is the NaOHX-themed mod manager injected into the client page: a
// floating button that opens a modal to upload/remove .jar mods and restart
// the local server. All text is set via textContent (mod names are untrusted).
const modUI = `
;(function () {
"use strict";
var API = "` + modsPath + `", RESTART = "` + backendRestartPath + `", H = { "X-EaglerCMP": "1" };
var css = ".nx-btn,.nx-modal,.nx-modal *{box-sizing:border-box;font-family:Consolas,'Courier New',monospace}" +
".nx-btn{position:fixed;right:14px;bottom:14px;z-index:2147483646;background:#0b0f14;color:#00e5ff;border:1px solid #00e5ff;border-radius:2px;padding:8px 14px;font-size:13px;letter-spacing:.08em;cursor:pointer;opacity:.55;transition:opacity .12s,box-shadow .12s}" +
".nx-btn:hover,.nx-btn:focus{opacity:1;box-shadow:0 0 12px rgba(0,229,255,.55);outline:none}" +
".nx-modal{position:fixed;inset:0;z-index:2147483647;background:rgba(5,8,12,.78);display:none;align-items:center;justify-content:center}" +
".nx-modal.open{display:flex}" +
".nx-card{width:min(560px,92vw);max-height:86vh;overflow:auto;background:#0b0f14;color:#d6e4ee;border:1px solid #1c2a36;border-top:2px solid #00e5ff;border-radius:2px;padding:18px}" +
".nx-card h2{margin:0 0 4px;font-size:16px;color:#00e5ff;letter-spacing:.12em;font-weight:700}" +
".nx-sub{font-size:11px;color:#6b8296;margin-bottom:12px}" +
".nx-pill{display:inline-block;padding:2px 8px;border:1px solid #1c2a36;font-size:11px;margin-bottom:10px;text-transform:uppercase;letter-spacing:.08em}" +
".nx-pill.ready{color:#b6ff3b;border-color:#b6ff3b}.nx-pill.starting{color:#ffd23b;border-color:#ffd23b}.nx-pill.error{color:#ff4d6d;border-color:#ff4d6d}" +
".nx-err{color:#ff4d6d;font-size:12px;margin:0 0 10px;white-space:pre-wrap;word-break:break-word}" +
".nx-drop{border:1px dashed #00e5ff;padding:18px;text-align:center;font-size:13px;color:#00e5ff;cursor:pointer;margin-bottom:12px}" +
".nx-drop.over,.nx-drop:hover{background:rgba(0,229,255,.08)}" +
".nx-row{display:flex;align-items:center;gap:8px;padding:6px 0;border-bottom:1px solid #121b24;font-size:12px}" +
".nx-row span:first-child{flex:1;word-break:break-all}.nx-row span+span{color:#6b8296}" +
".nx-b{background:transparent;color:#d6e4ee;border:1px solid #2a3b4a;border-radius:2px;padding:5px 10px;font-size:12px;cursor:pointer}" +
".nx-b:hover{border-color:#00e5ff;color:#00e5ff}.nx-b.pri{background:#00e5ff;color:#05080c;border-color:#00e5ff;font-weight:700}" +
".nx-foot{display:flex;gap:8px;justify-content:flex-end;margin-top:14px}.nx-msg{font-size:12px;color:#b6ff3b;min-height:16px;margin-top:8px}";
function el(t, c, x) { var e = document.createElement(t); if (c) e.className = c; if (x != null) e.textContent = x; return e; }
function init() {
	var st = document.createElement("style"); st.textContent = css; document.head.appendChild(st);
	var btn = el("button", "nx-btn", "NaOHX MODS"); btn.type = "button";
	var modal = el("div", "nx-modal"), card = el("div", "nx-card"); modal.appendChild(card);
	document.body.appendChild(btn); document.body.appendChild(modal);
	var input = el("input"); input.type = "file"; input.accept = ".jar"; input.multiple = true; input.style.display = "none";
	function api(url, opt) {
		opt = opt || {}; opt.headers = H;
		return fetch(url, opt).then(function (r) { return r.json().then(function (j) { if (!r.ok) throw new Error(j.error || r.statusText); return j; }); });
	}
	function render(data, msg) {
		card.textContent = "";
		card.appendChild(el("h2", null, "NaOHX // MODS"));
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
		api(API).then(function (d) { render(d, err || msg); if (err) card.querySelector(".nx-msg").style.color = "#ff4d6d"; },
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
