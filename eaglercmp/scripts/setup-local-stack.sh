#!/usr/bin/env sh
# Provision the local dual-layer stack under <instance> (idempotent):
#   backend/  Fabric game server (127.0.0.1:25566), your .jar mods go in backend/mods
#   gateway/  Velocity + EaglerXServer (+ ViaVersion/ViaBackwards): the Eaglercraft
#             WebSocket listener (same port as Velocity, 127.0.0.1:25565) that forwards standard
#             Minecraft packets to Fabric. The daemon tunnels /__eaglercmp/bridge to it.
# Env: MC_VERSION (default 1.21.1), VELOCITY_VERSION (default 3.4.0-SNAPSHOT).
# Running this accepts the Minecraft EULA (https://aka.ms/MinecraftEULA).
set -eu
inst="${1:?usage: setup-local-stack.sh <instance-dir>}"
mc="${MC_VERSION:-1.21.1}"
vel="${VELOCITY_VERSION:-3.4.0-SNAPSHOT}"
be="$inst/backend"; gw="$inst/gateway"
mkdir -p "$be/mods" "$gw/plugins"

fetch() { curl -fsSL --retry 3 -o "$2.part" "$1" && mv "$2.part" "$2"; }
# first file URL of the newest *release* Modrinth version (needs jq) of a project for a loader
modrinth() {
  curl -fsS "https://api.modrinth.com/v2/project/$1/version?loaders=%5B%22$2%22%5D" \
    | jq -r '[.[] | select(.version_type=="release")][0].files[0].url // empty'
}

# --- Fabric game server ---
if [ ! -f "$be/fabric-server-launch.jar" ]; then
  loader=$(curl -fsS https://meta.fabricmc.net/v2/versions/loader | sed -n 's/.*"version": "\([^"]*\)".*/\1/p' | head -1)
  installer=$(curl -fsS https://meta.fabricmc.net/v2/versions/installer | sed -n 's/.*"version": "\([^"]*\)".*/\1/p' | head -1)
  fetch "https://meta.fabricmc.net/v2/versions/loader/$mc/$loader/$installer/server/jar" "$be/fabric-server-launch.jar"
fi
echo "eula=true" > "$be/eula.txt"
[ -f "$be/server.properties" ] || printf 'online-mode=false\nserver-ip=127.0.0.1\nserver-port=25566\nenforce-secure-profile=false\n' > "$be/server.properties"

# --- Velocity gateway ---
if [ ! -f "$gw/velocity.jar" ]; then
  url=$(curl -fsS "https://fill.papermc.io/v3/projects/velocity/versions/$vel/builds/latest" | sed -n 's/.*"url":"\([^"]*\.jar\)".*/\1/p' | head -1)
  fetch "$url" "$gw/velocity.jar"
fi
# Loopback only, no accounts: Eaglercraft players have no Mojang session, and the
# Fabric server only listens on 127.0.0.1, so forwarding stays "none".
[ -f "$gw/velocity.toml" ] || cat > "$gw/velocity.toml" <<'TOML'
config-version = "2.7"
bind = "127.0.0.1:25565"
motd = "EaglerCMP local gateway"
online-mode = false
player-info-forwarding-mode = "none"
force-key-authentication = false

[servers]
fabric = "127.0.0.1:25566"
try = ["fabric"]

[forced-hosts]
TOML
for p in "eaglercraftxserver:EaglerXServer" "viaversion:ViaVersion" "viabackwards:ViaBackwards"; do
  slug=${p%%:*}; name=${p##*:}
  [ -f "$gw/plugins/$name.jar" ] && continue
  url=$(modrinth "$slug" velocity)
  [ -n "$url" ] && fetch "$url" "$gw/plugins/$name.jar" || echo "warning: could not download $name" >&2
done

# EaglerXServer injects its WebSocket listener into Velocity's socket by address and
# generates listeners.toml on first start: run Velocity once, then pin the address to
# loopback so the gateway is never exposed beyond this machine.
lt="$gw/plugins/eaglerxserver/listeners.toml"
if [ ! -f "$lt" ]; then
  (cd "$gw" && exec java -Xmx512M -jar velocity.jar >/dev/null 2>&1) &
  pid=$!
  i=0; while [ ! -f "$lt" ] && [ $i -lt 90 ]; do sleep 1; i=$((i+1)); done
  sleep 2; kill "$pid" 2>/dev/null || true; wait "$pid" 2>/dev/null || true
fi
[ -f "$lt" ] || { echo "error: EaglerXServer did not generate $lt" >&2; exit 1; }
sed -i 's/inject_address = "0.0.0.0:25565"/inject_address = "127.0.0.1:25565"/' "$lt"

# --- daemon config (only if missing) ---
cfg="$inst/eaglercmp.json"
[ -f "$cfg" ] || cat > "$cfg" <<JSON
{
  "port": 47262,
  "width": 1280,
  "height": 720,
  "performance": true,
  "backend": {"enabled": true, "name": "Local Modded Server", "jar": "fabric-server-launch.jar", "jvmArgs": ["-Xmx2G"], "port": 25566},
  "gateway": {"enabled": true, "jar": "velocity.jar", "jvmArgs": ["-Xmx512M"], "args": [], "port": 25565}
}
JSON
echo "local stack ready in $inst"
