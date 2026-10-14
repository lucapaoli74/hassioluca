#!/usr/bin/env python3
"""Trova la scheda ESP32 collegata via USB e stampa cosa c'è sopra.

Uso (sul PC a cui è collegata la scheda):
    pip install esptool pyserial
    python3 trova-scheda.py            # cerca su tutte le porte seriali
    python3 trova-scheda.py COM5       # oppure una porta precisa

Per ogni porta trovata stampa: chip (ESP32 / S3 / C3...), funzioni
(WiFi/BLE/PSRAM), dimensione flash, MAC e le prime righe di log al boot
(da lì si vede se c'è già un firmware ESPHome/Arduino e quale display/touch
inizializza). Incolla l'output nella chat per scegliere la configurazione
giusta del pannello.
"""
import subprocess
import sys
import time

try:
    import serial
    from serial.tools import list_ports
except ImportError:
    sys.exit("Manca pyserial: pip install esptool pyserial")

# VID:PID dei convertitori USB-seriale usati sulle schede ESP32
KNOWN_USB = {
    (0x10C4, 0xEA60): "CP210x (Silicon Labs)",
    (0x1A86, 0x7523): "CH340",
    (0x1A86, 0x55D4): "CH9102 / CH343",
    (0x0403, 0x6001): "FTDI FT232",
    (0x303A, 0x1001): "USB nativo ESP32-S3/C3 (USB-JTAG/serial)",
    (0x303A, 0x0002): "USB nativo ESP32-S2/S3 (TinyUSB)",
}


def candidate_ports(wanted):
    ports = list(list_ports.comports())
    if wanted:
        return [p for p in ports if p.device == wanted] or [
            type("P", (), {"device": wanted, "vid": None, "pid": None,
                           "description": "(indicata a mano)"})()]
    found = [p for p in ports if (p.vid, p.pid) in KNOWN_USB]
    return found or ports


def esptool_info(port):
    # flash_id stampa chip, features (incl. PSRAM integrata), MAC e flash
    cmd = [sys.executable, "-m", "esptool", "--port", port, "flash_id"]
    try:
        out = subprocess.run(cmd, capture_output=True, text=True, timeout=60)
    except subprocess.TimeoutExpired:
        return "  esptool: timeout (tieni premuto BOOT, premi RST, rilascia BOOT e riprova)"
    keep = ("Chip is", "Chip type", "Features", "Crystal", "MAC",
            "Detected flash size", "Flash size", "Manufacturer", "Device",
            "Embedded", "PSRAM", "error", "Error", "Failed")
    lines = [l for l in (out.stdout + out.stderr).splitlines()
             if any(k in l for k in keep)]
    return "\n".join("  " + l.strip() for l in lines) or "  esptool: nessuna risposta"


def boot_log(port, seconds=4):
    # reset via DTR/RTS (EN basso -> alto, IO0 alto) e legge il log di avvio
    try:
        with serial.Serial(port, 115200, timeout=0.2) as s:
            s.dtr = False
            s.rts = True
            time.sleep(0.1)
            s.rts = False
            end = time.time() + seconds
            buf = b""
            while time.time() < end:
                buf += s.read(1024)
    except serial.SerialException as e:
        return f"  porta occupata o non accessibile: {e}"
    text = buf.decode("utf-8", errors="replace").strip()
    lines = text.splitlines()[:40]
    return "\n".join("  | " + l for l in lines) or "  (nessun output al boot)"


def main():
    wanted = sys.argv[1] if len(sys.argv) > 1 else None
    ports = candidate_ports(wanted)
    if not ports:
        sys.exit("Nessuna porta seriale trovata. Cavo USB solo-alimentazione? "
                 "Driver CH340/CP210x installato?")
    for p in ports:
        usb = KNOWN_USB.get((p.vid, p.pid), p.description)
        print(f"=== {p.device}  [{usb}]")
        print(esptool_info(p.device))
        print("  --- log di avvio (115200 baud) ---")
        print(boot_log(p.device))
        print()


if __name__ == "__main__":
    main()
