# Notices and attribution

EaglerCMP is a desktop wrapper for the Eaglercraft web client. Its default
client is Eaglercraft 26.2 from github.com/eymenwsmc/playopspt.github.io
(`262/`). Release builds embed that client unmodified; plain builds download it
from the upstream repo on first launch. EaglerCMP stores the client locally,
verifies it by SHA-256 and serves it to a window on the loopback interface.
EaglerCMP's own code contains no Minecraft or Mojang code or assets.

"Sodium HX Graphics" / "NaOHX" is a software brand name used by EaglerCMP for
its window performance profile (browser-engine GPU and scheduling flags). It
is a marketing label for a software graphics pipeline only and has no chemical
meaning.

| Component | Author | License | Source |
|---|---|---|---|
| Eaglercraft (web client, project origin) | lax1dude & contributors | see upstream | https://github.com/eymenwsmc/playopspt.github.io |
| TeaVM (compiler used by Eaglercraft's Wasm-GC builds) | Alexey Andreev & contributors | Apache-2.0 | https://github.com/konsoletyper/teavm |
| Sodium ("Sodium" in the brand name) | CaffeineMC (JellySquid & contributors) | Polyform Shield 1.0.0 | https://github.com/CaffeineMC/sodium |

Thank you to CaffeineMC, whose Sodium project inspired the "Sodium HX
Graphics" name. EaglerCMP contains no CaffeineMC code; Sodium itself is a Java
Edition mod and is not used by the web client.

Eaglercraft client builds, including the one embedded in release binaries,
contain code and assets derived from Minecraft by Mojang Studios. They remain
under their original terms; EaglerCMP's license does not cover them.
