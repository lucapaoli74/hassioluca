#!/usr/bin/env bash
# Webcam Manager — made by Paoli Luca 2026
# Installa (o aggiorna) Webcam Manager su una VM o un PC Linux Debian/Ubuntu,
# ad esempio su Proxmox o VMware vCenter. Da eseguire come root:
#
#   curl -fsSL https://raw.githubusercontent.com/lucapaoli74/hassioluca/main/webcam-manager/deploy/install-linux.sh | sudo bash -s -- \
#       --location "Camping Coggiolo Sant'Anna Pelago (MO)" --altitude 1250 --password "una-password"
#
# Opzioni:
#   --location NOME     nome della località          --altitude METRI   altitudine s.l.m.
#   --password PWD      password del pannello         --port PORTA       porta (default 8080)
#   --binary FILE       usa un eseguibile già scaricato invece di scaricarlo
#   --url URL           scarica l'eseguibile da questo indirizzo
# Variabile GITHUB_TOKEN: necessaria se il repository è privato.
set -euo pipefail

REPO="lucapaoli74/hassioluca"
LOCATION="" ALTITUDE="" PASSWORD="" PORT="8080" BINARY="" URL=""
while [ $# -gt 0 ]; do
  case "$1" in
    --location) LOCATION="$2"; shift 2 ;;
    --altitude) ALTITUDE="$2"; shift 2 ;;
    --password) PASSWORD="$2"; shift 2 ;;
    --port) PORT="$2"; shift 2 ;;
    --binary) BINARY="$2"; shift 2 ;;
    --url) URL="$2"; shift 2 ;;
    *) echo "opzione sconosciuta: $1" >&2; exit 2 ;;
  esac
done
[ "$(id -u)" -eq 0 ] || { echo "Esegui come root (sudo)." >&2; exit 1; }

case "$(uname -m)" in
  x86_64) ARCH=amd64 ;; aarch64) ARCH=arm64 ;; armv7l|armv6l) ARCH=arm ;;
  *) echo "architettura non supportata: $(uname -m)" >&2; exit 1 ;;
esac

echo "==> Pacchetti di sistema (ffmpeg per le telecamere RTSP)"
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
apt-get install -y -qq ffmpeg ca-certificates curl python3 tzdata qemu-guest-agent open-vm-tools >/dev/null 2>&1 || apt-get install -y -qq ffmpeg ca-certificates curl python3

DEST=/opt/webcam-manager/webcam-manager
mkdir -p /opt/webcam-manager
TMP=$(mktemp)
if [ -n "$BINARY" ]; then
  cp "$BINARY" "$TMP"
else
  if [ -z "$URL" ]; then
    echo "==> Ricerca dell'ultima versione su GitHub"
    AUTH=()
    [ -n "${GITHUB_TOKEN:-}" ] && AUTH=(-H "Authorization: Bearer $GITHUB_TOKEN")
    URL=$(curl -fsSL "${AUTH[@]}" -H "Accept: application/vnd.github+json" "https://api.github.com/repos/$REPO/releases/latest" \
      | python3 -c "import json,sys; print(next((a['url'] for a in json.load(sys.stdin)['assets'] if a['name']=='webcam-manager-linux-$ARCH'), ''))") || true
    [ -n "$URL" ] || { echo "Eseguibile non trovato nell'ultima release (repository privato? imposta GITHUB_TOKEN, oppure usa --binary)." >&2; exit 1; }
    curl -fsSL "${AUTH[@]}" -H "Accept: application/octet-stream" -o "$TMP" "$URL"
  else
    curl -fsSL -o "$TMP" "$URL"
  fi
fi
chmod 755 "$TMP"
"$TMP" version >/dev/null || { echo "L'eseguibile scaricato non funziona." >&2; exit 1; }

if systemctl is-enabled webcam-manager >/dev/null 2>&1; then
  echo "==> Aggiornamento: sostituisco l'eseguibile, configurazione e storico restano"
  systemctl stop webcam-manager
  mv "$TMP" "$DEST"
  systemctl start webcam-manager
else
  mv "$TMP" "$DEST"
  ARGS=(install -data /var/lib/webcam-manager -listen ":$PORT")
  [ -n "$PASSWORD" ] && ARGS+=(-admin-password "$PASSWORD")
  [ -n "$LOCATION" ] && ARGS+=(-location "$LOCATION")
  [ -n "$ALTITUDE" ] && ARGS+=(-altitude "$ALTITUDE")
  "$DEST" "${ARGS[@]}"
fi

IP=$(hostname -I 2>/dev/null | awk '{print $1}')
echo
echo "Webcam Manager attivo: http://${IP:-indirizzo-della-vm}:$PORT  (utente admin)"
echo "Log: journalctl -u webcam-manager -f"
