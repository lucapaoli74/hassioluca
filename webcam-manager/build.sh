#!/usr/bin/env bash
# Compila Webcam Manager per tutte le piattaforme e prepara latest.json per
# gli aggiornamenti automatici.
#   ./build.sh 1.2.0 "Note di rilascio"
set -euo pipefail
cd "$(dirname "$0")"
VERSION="${1:-dev}"
NOTES="${2:-}"
OUT=dist
rm -rf "$OUT" && mkdir -p "$OUT"

targets=(
  "windows amd64 .exe"
  "linux amd64"
  "linux arm64"    # Raspberry Pi 4/5, mini PC ARM
  "linux arm"      # Raspberry Pi più vecchi
  "darwin amd64"
  "darwin arm64"
)
assets=""
for t in "${targets[@]}"; do
  read -r os arch ext <<<"$t"
  name="webcam-manager-$os-$arch${ext:-}"
  echo "→ $name"
  GOOS=$os GOARCH=$arch CGO_ENABLED=0 GOARM=7 go build -trimpath -ldflags "-s -w -X main.version=$VERSION" -o "$OUT/$name" .
  sum=$(sha256sum "$OUT/$name" | cut -d' ' -f1)
  [ -n "$assets" ] && assets+=","
  assets+="\"$os-$arch\":{\"file\":\"$name\",\"sha256\":\"$sum\"}"
done

python3 - "$VERSION" "$NOTES" "$assets" > "$OUT/latest.json" <<'PY'
import json, sys
version, notes, assets = sys.argv[1], sys.argv[2], json.loads("{" + sys.argv[3] + "}")
print(json.dumps({"version": version, "notes": notes, "assets": assets}, indent=2, ensure_ascii=False))
PY
echo "Fatto: $OUT/"
ls -la "$OUT"
