// Example EaglerCMP client mod. Copy to <instance>/client-mods/ and restart.
// Draws an FPS counter, toggles it with F8, logs bridge traffic and loads an optional asset.
window.eaglercmp.registerMod({
	id: "hello",
	name: "Hello example",
	version: "1.0.0",
	init: function (api) {
		var view = api.ui.canvas(), ctx = view.ctx, show = true, fps = 0;
		api.input.on("keydown", function (e) { if (e.code === "F8") show = !show; });
		api.render.onFrame(function (f) {
			if (f.dt) fps = fps * 0.9 + (1 / f.dt) * 0.1;
			ctx.clearRect(0, 0, view.canvas.width, view.canvas.height);
			if (show) { ctx.fillStyle = "#fff"; ctx.font = "14px monospace"; ctx.fillText("hello mod: " + fps.toFixed(0) + " fps (F8 toggles)", 10, view.canvas.height - 10); }
		});
		api.bridge.on("open", function () { api.log("bridge connected"); });
		api.bridge.on("message", function (m) { /* m.data is a binary Eaglercraft protocol frame */ });
		// Any file in client-assets/override/<path> already replaces matching requests; this adds a rule from code:
		// api.assets.override("/some/texture.png", "textures/mine.png");
		api.assets.list().forEach(function (p) { api.log("asset: " + p); });
	}
});
