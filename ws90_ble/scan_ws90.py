"""
Prova di ricezione Bluetooth del sensore Ecowitt WS90 "Powered by Shelly".

Ascolta per 60 secondi le trasmissioni Bluetooth vicine e mostra:
  - i dispositivi BTHome (il formato usato dai sensori Shelly) con i valori decodificati
  - l'elenco di tutti i dispositivi Bluetooth visti

Uso:
    pip install bleak
    python scan_ws90.py

Il risultato viene anche salvato in "scan_ws90_risultato.txt" nella stessa cartella.
"""

import asyncio
import os
import time

from bleak import BleakScanner

BTHOME_UUID = "0000fcd2-0000-1000-8000-00805f9b34fb"
DURATA_SECONDI = 60
PAROLE_CHIAVE = ("ws90", "shelly", "ecowitt", "wittboy", "sbws", "bthome")

# id oggetto BTHome v2 -> (nome, byte, con segno, fattore)
OGGETTI = {
    0x00: ("packet_id", 1, False, 1),
    0x01: ("batteria_%", 1, False, 1),
    0x02: ("temperatura_C", 2, True, 0.01),
    0x03: ("umidita_%", 2, False, 0.01),
    0x04: ("pressione_hPa", 3, False, 0.01),
    0x05: ("luce_lux", 3, False, 0.01),
    0x08: ("punto_rugiada_C", 2, True, 0.01),
    0x09: ("conteggio", 1, False, 1),
    0x0C: ("tensione_V", 2, False, 0.001),
    0x2E: ("umidita_%", 1, False, 1),
    0x3A: ("pulsante", 1, False, 1),
    0x3F: ("rotazione_gradi", 2, True, 0.1),
    0x44: ("velocita_m_s", 2, False, 0.01),
    0x45: ("temperatura_C", 2, True, 0.1),
    0x46: ("indice_UV", 1, False, 0.1),
    0x4A: ("tensione_V", 2, False, 0.1),
    0x57: ("temperatura_C", 1, True, 1),
    0x5E: ("direzione_gradi", 2, False, 0.01),
    0x5F: ("precipitazione_mm", 2, False, 0.1),
    0x60: ("canale", 1, False, 1),
    0xF0: ("tipo_dispositivo", 2, False, 1),
    0xF1: ("firmware", 4, False, 1),
    0xF2: ("firmware", 3, False, 1),
}
# dimensioni di altri oggetti noti, solo per poterli saltare
ALTRE_DIMENSIONI = {
    0x06: 2, 0x07: 2, 0x0A: 3, 0x0B: 3, 0x0D: 2, 0x0E: 2, 0x0F: 1, 0x10: 1,
    0x11: 1, 0x12: 2, 0x13: 2, 0x14: 2, 0x2F: 1, 0x3C: 2, 0x3D: 2, 0x3E: 4,
    0x40: 2, 0x41: 2, 0x42: 3, 0x43: 2, 0x47: 2, 0x48: 2, 0x49: 2, 0x4B: 3,
    0x4C: 4, 0x4D: 4, 0x4E: 4, 0x4F: 4, 0x50: 4, 0x51: 2, 0x52: 2, 0x55: 4,
    0x56: 2, 0x58: 1, 0x59: 1, 0x5A: 2, 0x5B: 4, 0x5C: 4, 0x5D: 2, 0x61: 2,
}
for _i in range(0x15, 0x2E):
    ALTRE_DIMENSIONI.setdefault(_i, 1)

righe = []


def scrivi(testo=""):
    print(testo)
    righe.append(testo)


def decodifica_bthome(dati: bytes):
    """Restituisce (info, lista di (nome, valore)) oppure solleva ValueError."""
    if not dati:
        raise ValueError("vuoto")
    info = dati[0]
    cifrato = bool(info & 0x01)
    versione = info >> 5
    descr = f"BTHome v{versione}, {'CIFRATO' if cifrato else 'non cifrato'}"
    if cifrato:
        return descr, []
    valori = []
    i = 1
    while i < len(dati):
        oid = dati[i]
        i += 1
        if oid in OGGETTI:
            nome, n, segno, fattore = OGGETTI[oid]
            grezzo = int.from_bytes(dati[i:i + n], "little", signed=segno)
            valore = grezzo * fattore
            valori.append((f"{nome} [0x{oid:02X}]", round(valore, 3)))
            i += n
        elif oid in ALTRE_DIMENSIONI:
            n = ALTRE_DIMENSIONI[oid]
            valori.append((f"oggetto 0x{oid:02X}", dati[i:i + n].hex()))
            i += n
        elif oid in (0x53, 0x54):  # testo / raw a lunghezza variabile
            n = dati[i]
            valori.append((f"oggetto 0x{oid:02X}", dati[i + 1:i + 1 + n].hex()))
            i += 1 + n
        else:
            valori.append((f"SCONOSCIUTO 0x{oid:02X}, resto", dati[i:].hex()))
            break
    return descr, valori


visti = {}          # indirizzo -> (nome, rssi)
ultimo_bthome = {}  # indirizzo -> bytes, per non ripetere pacchetti identici


def callback(device, adv):
    nome = adv.local_name or device.name or ""
    visti[device.address] = (nome, adv.rssi)

    dati = adv.service_data.get(BTHOME_UUID)
    interessante = any(p in nome.lower() for p in PAROLE_CHIAVE)
    if dati is None and not interessante:
        return
    if dati is not None and ultimo_bthome.get(device.address) == dati:
        return
    if dati is not None:
        ultimo_bthome[device.address] = dati

    ora = time.strftime("%H:%M:%S")
    scrivi(f"[{ora}] {device.address}  nome='{nome}'  segnale={adv.rssi} dBm")
    if dati is not None:
        scrivi(f"    dati grezzi: {dati.hex()}")
        try:
            descr, valori = decodifica_bthome(dati)
            scrivi(f"    {descr}")
            for n, v in valori:
                scrivi(f"      {n}: {v}")
        except Exception as e:  # noqa: BLE001
            scrivi(f"    errore di decodifica: {e}")
    else:
        for uuid, d in adv.service_data.items():
            scrivi(f"    service_data {uuid}: {d.hex()}")
        for cid, d in adv.manufacturer_data.items():
            scrivi(f"    manufacturer 0x{cid:04X}: {d.hex()}")
    scrivi()


async def main():
    scrivi(f"Ascolto il Bluetooth per {DURATA_SECONDI} secondi... "
           "tieni il WS90 acceso vicino al notebook.")
    scrivi()
    try:
        scanner = BleakScanner(detection_callback=callback)
        await scanner.start()
    except Exception as e:  # noqa: BLE001
        scrivi(f"ERRORE: non riesco ad avviare il Bluetooth: {e}")
        scrivi("Controlla che il Bluetooth del notebook sia acceso.")
        return
    await asyncio.sleep(DURATA_SECONDI)
    await scanner.stop()

    scrivi("=" * 60)
    scrivi(f"Dispositivi BTHome trovati: {len(ultimo_bthome)}")
    scrivi(f"Tutti i dispositivi Bluetooth visti ({len(visti)}):")
    for ind, (nome, rssi) in sorted(visti.items(), key=lambda x: -(x[1][1] or -999)):
        scrivi(f"  {ind}  segnale={rssi} dBm  nome='{nome}'")


if __name__ == "__main__":
    asyncio.run(main())
    percorso = os.path.join(os.path.dirname(os.path.abspath(__file__)),
                            "scan_ws90_risultato.txt")
    with open(percorso, "w", encoding="utf-8") as f:
        f.write("\n".join(righe))
    print(f"\nRisultato salvato in: {percorso}")
    input("Premi INVIO per chiudere...")
