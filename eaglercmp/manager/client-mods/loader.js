// EaglerCMP client-mod loader and plugin API (embedded in the launcher, injected into the game page head).
// Mods are .js / .wasm files in <instance>/client-mods, assets live in <instance>/client-assets.
// See eaglercmp/examples/client-mods/hello.js for a mod that uses every part of the API.
;(function () {
"use strict";
var MODS = "/__eaglercmp/client-mods", ASSETS = "/__eaglercmp/client-assets", BRIDGE = "/__eaglercmp/bridge";
var E = window.eaglercmp = window.eaglercmp || {};
var nativeFetch = window.fetch.bind(window);

// ---- event bus ------------------------------------------------------------
var bus = {};
function on(type, fn) {
	(bus[type] = bus[type] || []).push(fn);
	return function () { var a = bus[type] || [], i = a.indexOf(fn); if (i >= 0) a.splice(i, 1); };
}
function emit(type, data) {
	(bus[type] || []).slice().forEach(function (f) { try { f(data); } catch (e) { console.error("[EaglerCMP] handler for '" + type + "' failed", e); } });
}
E.on = on; E.emit = function (t, d) { emit(t, d || {}); };

// ---- input hooks (capture phase: mods see events before the game) ----------
var down = {};
["keydown", "keyup", "mousedown", "mouseup", "mousemove", "wheel", "contextmenu"].forEach(function (t) {
	window.addEventListener(t, function (e) {
		if (t === "keydown") down[e.code] = true; else if (t === "keyup") delete down[e.code];
		emit("input:" + t, e);
	}, true);
});
document.addEventListener("pointerlockchange", function () { emit("input:pointerlock", { locked: !!document.pointerLockElement }); });

// ---- frame loop (render hook) --------------------------------------------
var last = 0;
(function tick(now) {
	var dt = last ? (now - last) / 1000 : 0; last = now;
	emit("frame", { dt: dt, now: now });
	requestAnimationFrame(tick);
})(performance.now());

// ---- WebSocket bridge observation ----------------------------------------
var NativeWS = window.WebSocket, activeBridge = null;
function BridgeAwareWebSocket(url, protocols) {
	var ws = protocols === undefined ? new NativeWS(url) : new NativeWS(url, protocols);
	if (String(url).indexOf(BRIDGE) >= 0) {
		var send = ws.send;
		ws.send = function (data) { emit("bridge:send", { data: data, socket: ws }); return send.apply(ws, arguments); };
		ws.__rawSend = function (data) { return send.call(ws, data); };
		ws.addEventListener("open", function () { activeBridge = ws; emit("bridge:open", { socket: ws }); });
		ws.addEventListener("message", function (ev) { emit("bridge:message", { data: ev.data, socket: ws }); });
		ws.addEventListener("close", function (ev) { if (activeBridge === ws) activeBridge = null; emit("bridge:close", { code: ev.code, reason: ev.reason }); });
		ws.addEventListener("error", function () { emit("bridge:error", {}); });
	}
	return ws;
}
BridgeAwareWebSocket.prototype = NativeWS.prototype;
["CONNECTING", "OPEN", "CLOSING", "CLOSED"].forEach(function (k) { BridgeAwareWebSocket[k] = NativeWS[k]; });
window.WebSocket = BridgeAwareWebSocket;

// ---- asset overrides -------------------------------------------------------
// Files under client-assets/override/<path> replace any same-origin request whose URL path ends with <path>
// (fetch, XHR, <img>/<audio>/<video> src). Mods can add rules with api.assets.override().
// Only resources the page loads by URL can be replaced: textures packed inside the game's own payload are not reachable.
var assetList = [], rules = [];
var assetsReady = nativeFetch(ASSETS, { cache: "no-store" }).then(function (r) { return r.json(); }).then(function (d) {
	assetList = d.files || [];
	assetList.forEach(function (p) {
		if (p.indexOf("override/") === 0 && p.length > 9) {
			var tail = "/" + p.slice(9);
			rules.push({ test: function (path) { return path === tail || path.slice(-tail.length) === tail; }, to: assetUrl(p), owner: "assets" });
		}
	});
	if (assetList.length) console.log("[EaglerCMP] " + assetList.length + " client asset(s), " + rules.length + " override rule(s)");
}).catch(function () {});
function assetUrl(p) { return ASSETS + "/" + p.split("/").map(encodeURIComponent).join("/"); }
function redirect(url) {
	if (!rules.length) return url;
	try {
		var u = new URL(url, location.href);
		if (u.origin !== location.origin || u.pathname.indexOf("/__eaglercmp/") === 0) return url;
		for (var i = 0; i < rules.length; i++) if (rules[i].test(u.pathname, u)) { console.log("[EaglerCMP] asset override " + u.pathname + " -> " + rules[i].to); return rules[i].to; }
	} catch (e) {}
	return url;
}
window.fetch = function (input, init) {
	return assetsReady.then(function () {
		if (typeof input === "string" || input instanceof URL) return nativeFetch(redirect(String(input)), init);
		var to = redirect(input.url);
		return nativeFetch(to === input.url ? input : new Request(to, input), init);
	});
};
var xo = XMLHttpRequest.prototype.open;
XMLHttpRequest.prototype.open = function (m, url) { var a = Array.prototype.slice.call(arguments); a[1] = redirect(String(url)); return xo.apply(this, a); };
[HTMLImageElement, HTMLMediaElement].forEach(function (C) {
	var d = Object.getOwnPropertyDescriptor(C.prototype, "src"); if (!d || !d.set) return;
	Object.defineProperty(C.prototype, "src", { configurable: true, enumerable: d.enumerable, get: d.get, set: function (v) { d.set.call(this, redirect(String(v))); } });
});

// ---- UI overlay root -----------------------------------------------------
var uiRoot;
function root() {
	if (!uiRoot) {
		uiRoot = document.createElement("div");
		uiRoot.style.cssText = "position:fixed;inset:0;pointer-events:none;z-index:2147483100";
		document.body.appendChild(uiRoot);
	}
	return uiRoot;
}

// ---- mod registry ----------------------------------------------------------
var mods = {};
var ID_RE = /^[a-z0-9][a-z0-9._-]{0,63}$/;
function makeApi(id, cleanup) {
	var own = function (off) { cleanup.push(off); return off; };
	var panel;
	return {
		id: id,
		log: function () { console.log.apply(console, ["[" + id + "]"].concat([].slice.call(arguments))); },
		on: function (t, f) { return own(on(t, f)); },
		emit: function (t, d) { emit(t, d || {}); },
		input: {
			on: function (type, f) { return own(on("input:" + type, f)); },
			isDown: function (code) { return !!down[code]; }
		},
		render: {
			onFrame: function (f) { return own(on("frame", f)); },
			get camera() { return E.camera; }
		},
		ui: {
			// A div (pointer-events: none by default) layered above the game and private to this mod.
			panel: function () {
				if (!panel) { panel = document.createElement("div"); panel.dataset.mod = id; panel.style.cssText = "position:absolute;inset:0"; root().appendChild(panel); cleanup.push(function () { panel.remove(); }); }
				return panel;
			},
			// A full-window 2D canvas that tracks the window size.
			canvas: function () {
				var c = document.createElement("canvas"); c.style.cssText = "position:absolute;inset:0;width:100%;height:100%";
				function fit() { c.width = innerWidth; c.height = innerHeight; }
				fit(); window.addEventListener("resize", fit); cleanup.push(function () { window.removeEventListener("resize", fit); });
				this.panel().appendChild(c);
				return { canvas: c, ctx: c.getContext("2d") };
			}
		},
		bridge: {
			on: function (type, f) { return own(on("bridge:" + type, f)); }, // open | message | send | close | error
			// Raw send on the game's bridge socket. The stream is the Eaglercraft protocol: malformed frames will disconnect the player.
			send: function (data) { if (!activeBridge) return false; activeBridge.__rawSend(data); return true; }
		},
		assets: {
			list: function () { return assetList.slice(); },
			url: assetUrl,
			override: function (match, target) {
				var test = typeof match === "function" ? match : match instanceof RegExp ? function (p) { return match.test(p); } : function (p) { return p === match || p.slice(-match.length) === match; };
				var rule = { test: test, to: /^[a-z]+:|^\//.test(target) ? target : assetUrl(target), owner: id };
				rules.push(rule); return own(function () { var i = rules.indexOf(rule); if (i >= 0) rules.splice(i, 1); });
			},
			image: function (p) { return new Promise(function (ok, no) { var i = new Image(); i.onload = function () { ok(i); }; i.onerror = no; i.src = assetUrl(p); }); },
			audio: function (p) { return new Audio(assetUrl(p)); },
			json: function (p) { return nativeFetch(assetUrl(p)).then(function (r) { return r.json(); }); },
			bytes: function (p) { return nativeFetch(assetUrl(p)).then(function (r) { return r.arrayBuffer(); }); }
		}
	};
}
// registerMod({ id, name?, version?, init(api), dispose?() }) -> true when accepted.
E.registerMod = function (def) {
	if (!def || !ID_RE.test(def.id || "")) { console.error("[EaglerCMP] registerMod: id must match " + ID_RE); return false; }
	if (mods[def.id]) { console.warn("[EaglerCMP] mod '" + def.id + "' already registered"); return false; }
	var cleanup = [], m = mods[def.id] = { def: def, cleanup: cleanup, state: "loading", error: "" };
	try {
		if (typeof def.init === "function") def.init(makeApi(def.id, cleanup));
		m.state = "ready";
	} catch (e) { m.state = "error"; m.error = String(e); console.error("[EaglerCMP] mod '" + def.id + "' failed to init", e); E.unregisterMod(def.id, true); }
	emit("mod:registered", { id: def.id });
	return m.state === "ready";
};
E.unregisterMod = function (id, keep) {
	var m = mods[id]; if (!m) return;
	try { if (m.def.dispose) m.def.dispose(); } catch (e) { console.error(e); }
	m.cleanup.splice(0).forEach(function (f) { try { f(); } catch (e) {} });
	if (!keep) delete mods[id]; else m.state = "error";
};
E.mods = function () { return Object.keys(mods).map(function (k) { var m = mods[k]; return { id: k, name: m.def.name || k, version: m.def.version || "", state: m.state, error: m.error }; }); };

// ---- WASM mods ---------------------------------------------------------------
// Imports: env.log(ptr, len) prints a UTF-8 string from the module's exported memory.
// Optional exports: init(), frame(dt: f64), key(code: i32, down: i32), memory.
function loadWasm(name) {
	var id = name.replace(/\.wasm$/, "").toLowerCase().replace(/[^a-z0-9._-]/g, "-");
	var inst;
	var imports = { env: { log: function (p, n) { if (inst && inst.exports.memory) console.log("[" + id + "]", new TextDecoder().decode(new Uint8Array(inst.exports.memory.buffer, p, n))); } } };
	return WebAssembly.instantiateStreaming(nativeFetch(MODS + "/" + encodeURIComponent(name), { cache: "no-store" }), imports).then(function (r) {
		inst = r.instance;
		E.registerMod({ id: id, name: name, init: function (api) {
			var x = inst.exports;
			if (x.init) x.init();
			if (x.frame) api.render.onFrame(function (f) { x.frame(f.dt); });
			if (x.key) { api.input.on("keydown", function (e) { x.key(e.keyCode, 1); }); api.input.on("keyup", function (e) { x.key(e.keyCode, 0); }); }
		} });
	});
}

// ---- startup: load every mod after the game page has loaded ----------------------
function boot() {
	nativeFetch(MODS, { cache: "no-store" }).then(function (r) { return r.json(); }).then(function (d) {
		var names = d.mods || [];
		return assetsReady.then(function () {
			return names.reduce(function (p, n) {
				return p.then(function () {
					if (/\.wasm$/.test(n)) return loadWasm(n).catch(function (e) { console.error("[EaglerCMP] wasm mod " + n + " failed", e); });
					return new Promise(function (ok) {
						var s = document.createElement("script");
						s.src = MODS + "/" + encodeURIComponent(n);
						s.onload = function () { ok(); };
						s.onerror = function () { console.error("[EaglerCMP] could not load client mod " + n); ok(); };
						document.body.appendChild(s);
					});
				});
			}, Promise.resolve());
		}).then(function () { emit("mods:loaded", { mods: E.mods() }); console.log("[EaglerCMP] client mods: " + E.mods().length + " registered"); });
	}).catch(function (e) { console.warn("[EaglerCMP] client mods unavailable", e); });
}
if (document.readyState === "complete") boot(); else window.addEventListener("load", boot);
})();
