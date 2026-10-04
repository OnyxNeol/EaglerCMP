#!/usr/bin/env sh
# Prepare a Fabric test server in <instance>/backend and enable it in eaglercmp.json.
#   MC_VERSION=1.21.1 EAGLER_PLUGIN_URL=<url of an Eaglercraft-protocol server mod .jar> scripts/testserver.sh <instance-dir>
# Fabric itself is downloaded from meta.fabricmc.net. The Eaglercraft WebSocket
# plugin is NOT bundled: EaglerXServer targets Spigot/BungeeCord/Velocity, so
# supply a Fabric-compatible Eaglercraft bridge via EAGLER_PLUGIN_URL (or drop it
# into <instance>/backend/mods yourself, or upload it from the NaOHX MODS button).
set -eu
inst="${1:?usage: testserver.sh <instance-dir>}"
mc="${MC_VERSION:-1.21.1}"
dir="$inst/backend"
mkdir -p "$dir/mods"
loader=$(curl -fsS https://meta.fabricmc.net/v2/versions/loader | sed -n 's/.*"version": "\([^"]*\)".*/\1/p' | head -1)
installer=$(curl -fsS https://meta.fabricmc.net/v2/versions/installer | sed -n 's/.*"version": "\([^"]*\)".*/\1/p' | head -1)
curl -fsSL -o "$dir/fabric-server-launch.jar" \
  "https://meta.fabricmc.net/v2/versions/loader/$mc/$loader/$installer/server/jar"
echo "eula=true" > "$dir/eula.txt"   # by running this script you accept https://aka.ms/MinecraftEULA
printf 'online-mode=false\nserver-port=25565\n' > "$dir/server.properties"
if [ -n "${EAGLER_PLUGIN_URL:-}" ]; then
  curl -fsSL -o "$dir/mods/eaglercraft-bridge.jar" "$EAGLER_PLUGIN_URL"
else
  echo "warning: no EAGLER_PLUGIN_URL; the server will start but nothing will listen for the web client." >&2
fi
cfg="$inst/eaglercmp.json"
echo "Fabric $mc ready in $dir. Enable it by setting in $cfg:"
echo '  "backend": {"enabled": true, "jar": "fabric-server-launch.jar", "jvmArgs": ["-Xmx2G"]}'
