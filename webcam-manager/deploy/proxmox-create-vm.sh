#!/usr/bin/env bash
# Crea su Proxmox VE una VM Debian 12 con Webcam Manager già installato.
# Da eseguire sull'host Proxmox (shell del nodo) come root:
#
#   bash proxmox-create-vm.sh --vmid 150 --bridge vmbr0 --storage local-lvm \
#       --location "Camping Coggiolo Sant'Anna Pelago (MO)" --altitude 1250 --password "una-password"
#
# La VM usa la rete in bridge: deve stare nella stessa LAN delle telecamere,
# altrimenti la ricerca automatica non le trova. Indirizzo via DHCP (o --ip).
set -euo pipefail

VMID=150 NAME=webcam-manager BRIDGE=vmbr0 STORAGE=local-lvm SNIPPETS=local
MEM=2048 CORES=2 DISK=32G IP=dhcp GW="" VLAN=""
LOCATION="" ALTITUDE=0 PASSWORD="" PORT=8080
while [ $# -gt 0 ]; do
  case "$1" in
    --vmid) VMID="$2"; shift 2 ;; --name) NAME="$2"; shift 2 ;;
    --bridge) BRIDGE="$2"; shift 2 ;; --vlan) VLAN="$2"; shift 2 ;;
    --storage) STORAGE="$2"; shift 2 ;; --snippets) SNIPPETS="$2"; shift 2 ;;
    --memory) MEM="$2"; shift 2 ;; --cores) CORES="$2"; shift 2 ;; --disk) DISK="$2"; shift 2 ;;
    --ip) IP="$2"; shift 2 ;; --gw) GW="$2"; shift 2 ;;
    --location) LOCATION="$2"; shift 2 ;; --altitude) ALTITUDE="$2"; shift 2 ;;
    --password) PASSWORD="$2"; shift 2 ;; --port) PORT="$2"; shift 2 ;;
    *) echo "opzione sconosciuta: $1" >&2; exit 2 ;;
  esac
done
command -v qm >/dev/null || { echo "Questo script va eseguito sull'host Proxmox." >&2; exit 1; }
[ -n "$PASSWORD" ] || { echo "Indica --password per il pannello." >&2; exit 1; }
qm status "$VMID" >/dev/null 2>&1 && { echo "La VM $VMID esiste già." >&2; exit 1; }

IMG=/var/lib/vz/template/iso/debian-12-genericcloud-amd64.qcow2
if [ ! -f "$IMG" ]; then
  echo "==> Scarico l'immagine cloud di Debian 12"
  mkdir -p "$(dirname "$IMG")"
  wget -q --show-progress -O "$IMG.part" https://cloud.debian.org/images/cloud/bookworm/latest/debian-12-genericcloud-amd64.qcow2
  mv "$IMG.part" "$IMG"
fi

# il cloud-init di Proxmox accetta "snippets" su uno storage che li abiliti
SNIPDIR=$(pvesm path "$SNIPPETS:snippets/x" 2>/dev/null | sed 's#/x$##') || true
if [ -z "$SNIPDIR" ]; then
  echo "Abilita il contenuto 'Snippets' sullo storage $SNIPPETS (Datacenter → Storage → Modifica)." >&2; exit 1
fi
mkdir -p "$SNIPDIR"
q() { printf "%q" "$1"; }
cat > "$SNIPDIR/webcam-manager-$VMID.yaml" <<YAML
#cloud-config
hostname: $NAME
timezone: Europe/Rome
package_update: true
packages: [curl, ffmpeg, qemu-guest-agent]
runcmd:
  - [ systemctl, enable, --now, qemu-guest-agent ]
  - curl -fsSL https://raw.githubusercontent.com/lucapaoli74/hassioluca/main/webcam-manager/deploy/install-linux.sh | bash -s -- --location $(q "$LOCATION") --altitude $(q "$ALTITUDE") --password $(q "$PASSWORD") --port $PORT
YAML

NET="virtio,bridge=$BRIDGE"; [ -n "$VLAN" ] && NET="$NET,tag=$VLAN"
IPCFG="ip=$IP"; [ "$IP" != dhcp ] && [ -n "$GW" ] && IPCFG="$IPCFG,gw=$GW"

echo "==> Creo la VM $VMID ($NAME)"
qm create "$VMID" --name "$NAME" --memory "$MEM" --cores "$CORES" --cpu host --net0 "$NET" \
  --scsihw virtio-scsi-single --ostype l26 --agent enabled=1 --onboot 1 --serial0 socket --vga serial0
qm importdisk "$VMID" "$IMG" "$STORAGE" >/dev/null
qm set "$VMID" --scsi0 "$STORAGE:vm-$VMID-disk-0,discard=on" --boot order=scsi0 >/dev/null
qm resize "$VMID" scsi0 "$DISK" >/dev/null
qm set "$VMID" --ide2 "$STORAGE:cloudinit" --ipconfig0 "$IPCFG" --cicustom "user=$SNIPPETS:snippets/webcam-manager-$VMID.yaml" >/dev/null
qm start "$VMID"

echo
echo "VM avviata: l'installazione automatica richiede qualche minuto."
echo "Indirizzo: qm guest cmd $VMID network-get-interfaces   (oppure dalla console)"
echo "Pannello:  http://INDIRIZZO-VM:$PORT  (utente admin)"
echo "La VM parte da sola all'accensione dell'host (onboot=1)."
