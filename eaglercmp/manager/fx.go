package manager

// fxRuntime is the built-in client effects runtime, injected into the page head:
//   - eaglercmp.camera: view rotation / projection captured from the game's
//     uniform-buffer uploads (the game exposes no camera API; its WebGL2 calls
//     go through the context prototype, so they can be observed).
//   - eaglercmp.on/emit: event bus fed by the launcher's /__eaglercmp/events stream.
//   - a physics-debris proof of concept (overlay canvas + Web Worker) for "blockBreak".
const fxRuntime = `
;(function () {
"use strict";
var E = window.eaglercmp = window.eaglercmp || {}; // on/emit come from client-mods/loader.js, injected before this
// ---- camera capture -------------------------------------------------------
var cam = E.camera = { view: null, proj: null, viewport: [0, 0], at: 0 };
function unit(f, o) { var l = f[o] * f[o] + f[o + 1] * f[o + 1] + f[o + 2] * f[o + 2]; return Math.abs(l - 1) < 0.01; }
function scan(d) {
	var f = new Float32Array(d.buffer, d.byteOffset, d.byteLength >> 2);
	for (var i = 0; i + 16 <= f.length; i += 4) {
		var ok = true; for (var k = 0; k < 16; k++) if (f[i + k] !== f[i + k]) { ok = false; break; }
		if (!ok) continue;
		// column-major rotation: orthonormal columns, last column (0,0,0,1)
		if (f[i + 3] === 0 && f[i + 7] === 0 && f[i + 11] === 0 && f[i + 15] === 1 && unit(f, i) && unit(f, i + 4) && unit(f, i + 8) &&
			Math.abs(f[i] * f[i + 4] + f[i + 1] * f[i + 5] + f[i + 2] * f[i + 6]) < 0.01) {
			cam.view = Array.prototype.slice.call(f, i, i + 16); cam.at = performance.now();
		}
		// perspective projection: m[11] == -1, m[15] == 0
		if (f[i + 11] === -1 && f[i + 15] === 0 && f[i] > 0.1 && f[i + 5] > 0.1) cam.proj = Array.prototype.slice.call(f, i, i + 16);
	}
}
["WebGL2RenderingContext"].forEach(function (n) {
	var P = window[n] && window[n].prototype; if (!P) return;
	var sub = P.bufferSubData;
	P.bufferSubData = function (target, off, data) {
		if (target === 0x8A11 && data && data.buffer && data.byteLength >= 64 && data.byteLength <= 1024) { try { scan(data); } catch (e) {} }
		return sub.apply(this, arguments);
	};
});
// ---- event stream ---------------------------------------------------------
function connect() {
	var es = new EventSource("` + eventsPath + `");
	es.onmessage = function (m) { try { var d = JSON.parse(m.data); E.emit(d.type, d); } catch (e) {} };
}
if (window.EventSource) connect();
// ---- debris proof of concept ----------------------------------------------
var overlay, ctx, worker, parts = [];
var WORKER = "var P=[];onmessage=function(e){var m=e.data;if(m.add){P=P.concat(m.add)}};" +
	"setInterval(function(){var dt=1/60,out=[];P=P.filter(function(p){p.vy-=20*dt;p.x+=p.vx*dt;p.y+=p.vy*dt;p.z+=p.vz*dt;" +
	"if(p.y<p.floor){p.y=p.floor;p.vy*=-.45;p.vx*=.7;p.vz*=.7}p.t+=dt;if(p.t<p.life)out.push(p.x,p.y,p.z,p.c,p.s,1-p.t/p.life);return p.t<p.life});" +
	"postMessage(new Float32Array(out))},16);";
function ensure() {
	if (overlay) return;
	overlay = document.createElement("canvas");
	overlay.style.cssText = "position:fixed;inset:0;width:100%;height:100%;pointer-events:none;z-index:2147483000";
	document.body.appendChild(overlay); ctx = overlay.getContext("2d");
	worker = new Worker(URL.createObjectURL(new Blob([WORKER], { type: "text/javascript" })));
	worker.onmessage = function (e) { var had = parts.length; parts = e.data; if (parts.length || had) draw(); };
}
function project(x, y, z) {
	var v = cam.view, vx = x, vy = y, vz = z;
	if (v) { vx = v[0] * x + v[4] * y + v[8] * z; vy = v[1] * x + v[5] * y + v[9] * z; vz = v[2] * x + v[6] * y + v[10] * z; }
	if (vz >= -0.05) return null;
	var w = overlay.width, h = overlay.height, p = cam.proj;
	var fx = p ? p[0] : (1 / Math.tan(35 * Math.PI / 180)) / (w / h), fy = p ? p[5] : 1 / Math.tan(35 * Math.PI / 180);
	return [(0.5 + 0.5 * fx * vx / -vz) * w, (0.5 - 0.5 * fy * vy / -vz) * h, -vz];
}
function draw() {
	overlay.width = innerWidth; overlay.height = innerHeight; ctx.clearRect(0, 0, overlay.width, overlay.height);
	for (var i = 0; i + 6 <= parts.length; i += 6) {
		var s = project(parts[i], parts[i + 1], parts[i + 2]); if (!s) continue;
		var c = parts[i + 3] | 0, size = Math.max(2, parts[i + 4] * overlay.height * 0.9 / s[2]);
		ctx.globalAlpha = Math.min(1, parts[i + 5] * 2);
		ctx.fillStyle = "rgb(" + (c >> 16 & 255) + "," + (c >> 8 & 255) + "," + (c & 255) + ")";
		ctx.fillRect(s[0] - size / 2, s[1] - size / 2, size, size);
	}
}
function color(name) { var h = 7; for (var i = 0; i < (name || "").length; i++) h = (h * 31 + name.charCodeAt(i)) | 0; return 0x606060 + (Math.abs(h) % 0x9f9f9f & 0xe0e0e0); }
// blockBreak: {x,y,z, px,py,pz (player eye), block}. Without px/py/pz (or camera
// data) the debris spawns in front of the screen centre.
E.on("blockBreak", function (d) {
	if (!document.body) return;
	ensure();
	var rx = 0, ry = -0.3, rz = -3;
	if (typeof d.px === "number" && typeof d.x === "number") { rx = d.x + 0.5 - d.px; ry = d.y + 0.5 - d.py; rz = d.z + 0.5 - d.pz; }
	var add = [], c = color(d.block);
	for (var i = 0; i < 24; i++) add.push({ x: rx, y: ry, z: rz, vx: (Math.random() - .5) * 4, vy: Math.random() * 4 + 1, vz: (Math.random() - .5) * 4, floor: ry - 0.5, c: c, s: 0.06 + Math.random() * 0.06, t: 0, life: 2.5 });
	worker.postMessage({ add: add });
});
})();
`
