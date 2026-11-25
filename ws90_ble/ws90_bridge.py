"""
Ponte Bluetooth -> Webcam Manager per il sensore Ecowitt WS90 "Powered by Shelly".

Il WS90 (versione Shelly, nome Bluetooth "SBWS-90CM") trasmette i dati in Bluetooth
nel formato aperto BTHome v2. Questo programma li ascolta e ogni 60 secondi li invia a
Webcam Manager nello stesso formato che userebbe un gateway Ecowitt (protocollo
"Ecowitt", POST su /data/report/).

Uso:
    python ws90_bridge.py            -> funzionamento normale (invia a Webcam Manager)
    python ws90_bridge.py --prova    -> mostra i dati che invierebbe, senza inviarli

Requisiti: Python 3 e la libreria bleak (python -m pip install bleak).
"""

import asyncio
import hashlib
import json
import logging
import logging.handlers
import os
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
from datetime import datetime, timezone

# ---------------------------------------------------------------------------
# IMPOSTAZIONI (modificabili)
# ---------------------------------------------------------------------------
# Indirizzo di Webcam Manager. Se il programma gira sullo stesso PC di Webcam
# Manager va bene 127.0.0.1; altrimenti metti l'IP del PC, es. 192.168.1.125.
URL_WEBCAM_MANAGER = "http://127.0.0.1:8080/data/report/"

# Indirizzo Bluetooth del WS90 (letto con scan_ws90.py). Se lo lasci vuoto ("")
# il programma usa il primo dispositivo il cui nome inizia con "SBWS-90".
MAC_WS90 = "E4:0A:75:B6:CE:BB"

INTERVALLO_INVIO_SECONDI = 60

# Altitudine del sensore in metri: serve solo per la pressione "relativa"
# (riportata al livello del mare). Con 0 la relativa e' uguale all'assoluta.
ALTITUDINE_M = 0

# Se non arrivano dati dal WS90 da piu' di questo tempo, non invia nulla.
DATI_VECCHI_SECONDI = 300
# ---------------------------------------------------------------------------

VERSIONE = "1.0"
CARTELLA = os.path.dirname(os.path.abspath(__file__))
FILE_STATO = os.path.join(CARTELLA, "ws90_stato.json")
FILE_LOG = os.path.join(CARTELLA, "ws90_bridge.log")
BTHOME_UUID = "0000fcd2-0000-1000-8000-00805f9b34fb"

log = logging.getLogger("ws90")


# ---------------------------------------------------------------------------
# Decodifica BTHome v2
# ---------------------------------------------------------------------------
# id -> (chiave, byte, con segno, fattore)
OGGETTI = {
    0x00: ("packet_id", 1, False, 1),
    0x01: ("batteria", 1, False, 1),
    0x02: ("temperatura", 2, True, 0.01),
    0x03: ("umidita", 2, False, 0.01),
    0x04: ("pressione", 3, False, 0.01),
    0x05: ("lux", 3, False, 0.01),
    0x08: ("rugiada", 2, True, 0.01),
    0x0C: ("tensione", 2, False, 0.001),
    0x20: ("pioggia_rilevata", 1, False, 1),
    0x2E: ("umidita", 1, False, 1),
    0x44: ("velocita", 2, False, 0.01),
    0x45: ("temperatura", 2, True, 0.1),
    0x46: ("uv", 1, False, 0.1),
    0x5E: ("direzione", 2, False, 0.01),
    0x5F: ("pioggia_mm", 2, False, 0.1),
}
DIMENSIONI = {
    0x06: 2, 0x07: 2, 0x09: 1, 0x0A: 3, 0x0B: 3, 0x0D: 2, 0x0E: 2, 0x12: 2,
    0x13: 2, 0x14: 2, 0x3A: 1, 0x3C: 2, 0x3D: 2, 0x3E: 4, 0x3F: 2, 0x40: 2,
    0x41: 2, 0x42: 3, 0x43: 2, 0x47: 2, 0x48: 2, 0x49: 2, 0x4A: 2, 0x4B: 3,
    0x4C: 4, 0x4D: 4, 0x4E: 4, 0x4F: 4, 0x50: 4, 0x51: 2, 0x52: 2, 0x55: 4,
    0x56: 2, 0x57: 1, 0x58: 1, 0x59: 1, 0x5A: 2, 0x5B: 4, 0x5C: 4, 0x5D: 2,
    0x60: 1, 0x61: 2, 0xF0: 2, 0xF1: 4, 0xF2: 3,
}
for _i in range(0x0F, 0x2E):
    DIMENSIONI.setdefault(_i, 1)


def decodifica_bthome(dati: bytes) -> dict:
    """Decodifica un pacchetto BTHome v2 non cifrato.

    Le velocita' (0x44) possono comparire due volte: vengono raccolte in una lista.
    """
    if not dati or dati[0] & 0x01:
        raise ValueError("pacchetto vuoto o cifrato")
    risultato = {"velocita": []}
    i = 1
    while i < len(dati):
        oid = dati[i]
        i += 1
        if oid in OGGETTI:
            chiave, n, segno, fattore = OGGETTI[oid]
            if i + n > len(dati):
                raise ValueError("pacchetto troncato")
            valore = int.from_bytes(dati[i:i + n], "little", signed=segno) * fattore
            i += n
            if chiave == "velocita":
                risultato["velocita"].append(valore)
            else:
                risultato[chiave] = valore
        elif oid in DIMENSIONI:
            i += DIMENSIONI[oid]
        elif oid in (0x53, 0x54):
            i += 1 + dati[i]
        else:
            break  # oggetto sconosciuto: ci fermiamo, teniamo quanto letto
    if not risultato["velocita"]:
        del risultato["velocita"]
    return risultato


# ---------------------------------------------------------------------------
# Stato: ultimi valori ricevuti + contatori pioggia
# ---------------------------------------------------------------------------
class Stato:
    def __init__(self):
        self.valori = {}           # ultimi valori letti dal sensore
        self.ultimo_ricevuto = 0   # time.time() dell'ultimo pacchetto
        self.ultimo_packet_id = None
        # pioggia (mm)
        self.pioggia_grezza = None   # ultimo contatore letto dal sensore
        self.pioggia_totale = 0.0    # totale accumulato da noi (sopravvive ai riavvii)
        self.inizi = {}              # periodo -> [chiave_periodo, totale_all_inizio]
        self.evento_inizio = None
        self.ultima_pioggia = 0.0    # time.time() dell'ultimo aumento
        self.incrementi = []         # [(time, mm)] ultime 24 ore
        self.carica()

    # --- persistenza -------------------------------------------------------
    def carica(self):
        try:
            with open(FILE_STATO, encoding="utf-8") as f:
                d = json.load(f)
            self.pioggia_grezza = d.get("pioggia_grezza")
            self.pioggia_totale = d.get("pioggia_totale", 0.0)
            self.inizi = d.get("inizi", {})
            self.evento_inizio = d.get("evento_inizio")
            self.ultima_pioggia = d.get("ultima_pioggia", 0.0)
            self.incrementi = [tuple(x) for x in d.get("incrementi", [])]
        except FileNotFoundError:
            pass
        except Exception as e:  # noqa: BLE001
            log.warning("Stato non leggibile, riparto da zero: %s", e)

    def salva(self):
        d = {
            "pioggia_grezza": self.pioggia_grezza,
            "pioggia_totale": round(self.pioggia_totale, 2),
            "inizi": self.inizi,
            "evento_inizio": self.evento_inizio,
            "ultima_pioggia": self.ultima_pioggia,
            "incrementi": self.incrementi,
        }
        tmp = FILE_STATO + ".tmp"
        try:
            with open(tmp, "w", encoding="utf-8") as f:
                json.dump(d, f)
            os.replace(tmp, FILE_STATO)
        except Exception as e:  # noqa: BLE001
            log.warning("Non riesco a salvare lo stato: %s", e)

    # --- aggiornamento -----------------------------------------------------
    def aggiorna(self, dati: dict, adesso: float):
        pid = dati.get("packet_id")
        if pid is not None and pid == self.ultimo_packet_id:
            return False  # stesso pacchetto ripetuto
        self.ultimo_packet_id = pid
        self.ultimo_ricevuto = adesso
        for k, v in dati.items():
            if k not in ("packet_id", "pioggia_mm"):
                self.valori[k] = v
        if "pioggia_mm" in dati:
            self._pioggia(dati["pioggia_mm"], adesso)
        return True

    def _pioggia(self, grezza: float, adesso: float):
        grezza = round(grezza, 1)
        if self.pioggia_grezza is None:
            incremento = 0.0
        elif grezza >= self.pioggia_grezza:
            incremento = grezza - self.pioggia_grezza
        elif self.pioggia_grezza > 6000 and grezza < 500:
            incremento = grezza + 6553.6 - self.pioggia_grezza   # contatore ripartito da 0
        else:
            incremento = grezza   # sensore riavviato: contatore ripartito da 0
        if incremento > 100:      # valore assurdo: lo ignoriamo
            log.warning("Salto di pioggia anomalo (%.1f mm) ignorato", incremento)
            incremento = 0.0
        self.pioggia_grezza = grezza
        self._periodi(adesso)   # apre giorno/settimana/... nuovi PRIMA di sommare
        if incremento > 0:
            if adesso - self.ultima_pioggia > 24 * 3600 or self.evento_inizio is None:
                self.evento_inizio = self.pioggia_totale
            self.pioggia_totale += incremento
            self.ultima_pioggia = adesso
            self.incrementi.append((adesso, round(incremento, 2)))
        self.incrementi = [x for x in self.incrementi if adesso - x[0] <= 24 * 3600]

    # --- calcoli pioggia ---------------------------------------------------
    def _da_inizio(self, periodo: str, chiave: str) -> float:
        corrente = self.inizi.get(periodo)
        if corrente is None or corrente[0] != chiave:
            self.inizi[periodo] = [chiave, self.pioggia_totale]
            return 0.0
        return max(0.0, self.pioggia_totale - corrente[1])

    def _chiavi_periodi(self, adesso: float) -> dict:
        locale = datetime.fromtimestamp(adesso)   # ora locale del PC (Italia)
        iso = locale.isocalendar()
        return {
            "giorno": locale.strftime("%Y-%m-%d"),
            "settimana": f"{iso[0]}-W{iso[1]}",
            "mese": locale.strftime("%Y-%m"),
            "anno": locale.strftime("%Y"),
        }

    def _periodi(self, adesso: float):
        for periodo, chiave in self._chiavi_periodi(adesso).items():
            self._da_inizio(periodo, chiave)

    def pioggia(self, adesso: float) -> dict:
        k = self._chiavi_periodi(adesso)
        ultimi = lambda secondi: sum(mm for t, mm in self.incrementi if adesso - t <= secondi)  # noqa: E731
        if adesso - self.ultima_pioggia > 24 * 3600:
            evento = 0.0
        else:
            inizio = self.pioggia_totale if self.evento_inizio is None else self.evento_inizio
            evento = max(0.0, self.pioggia_totale - inizio)
        return {
            "rate": ultimi(600) * 6,   # mm/h, dalla pioggia degli ultimi 10 minuti
            "event": evento,
            "hourly": ultimi(3600),
            "daily": self._da_inizio("giorno", k["giorno"]),
            "weekly": self._da_inizio("settimana", k["settimana"]),
            "monthly": self._da_inizio("mese", k["mese"]),
            "yearly": self._da_inizio("anno", k["anno"]),
            "total": self.pioggia_totale,
        }


# ---------------------------------------------------------------------------
# Composizione del messaggio in formato Ecowitt
# ---------------------------------------------------------------------------
def _f(x, cifre=2):
    return f"{x:.{cifre}f}"


def componi_ecowitt(stato: Stato, adesso: float) -> dict:
    v = stato.valori
    mac = (MAC_WS90 or "WS90-BLE").upper()
    campi = {
        "PASSKEY": hashlib.md5(mac.encode()).hexdigest().upper(),
        "stationtype": f"WS90BLE_bridge_V{VERSIONE}",
        "dateutc": datetime.fromtimestamp(adesso, timezone.utc).strftime("%Y-%m-%d %H:%M:%S"),
        "freq": "868M",
        "model": "WS90_BLE",
    }
    if "temperatura" in v:
        campi["tempf"] = _f(v["temperatura"] * 9 / 5 + 32, 1)
    if "umidita" in v:
        campi["humidity"] = str(round(v["umidita"]))
    if "pressione" in v:
        hpa = v["pressione"]
        rel = hpa * (1 - 0.0065 * ALTITUDINE_M / 288.15) ** -5.255 if ALTITUDINE_M else hpa
        campi["baromabsin"] = _f(hpa * 0.0295300, 3)
        campi["baromrelin"] = _f(rel * 0.0295300, 3)
    if "direzione" in v:
        campi["winddir"] = str(round(v["direzione"]) % 360)
    if v.get("velocita"):
        # il WS90 manda due velocita': vento medio e raffica. Il vento medio non
        # puo' superare la raffica, quindi prendiamo il minore e il maggiore.
        medio, raffica = min(v["velocita"]), max(v["velocita"])
        campi["windspeedmph"] = _f(medio * 2.23694)
        campi["windgustmph"] = _f(raffica * 2.23694)
    if "lux" in v:
        campi["solarradiation"] = _f(v["lux"] / 126.7)   # stessa conversione di Ecowitt
    if "uv" in v:
        campi["uv"] = str(round(v["uv"]))
    if "batteria" in v:
        # Ecowitt per il WS90 usa la tensione delle pile: 100% ~ 3.2 V
        campi["wh90batt"] = _f(2.4 + 0.8 * v["batteria"] / 100)
    if "tensione" in v:
        campi["ws90cap_volt"] = _f(v["tensione"], 1)

    p = stato.pioggia(adesso)
    pollici = {k: _f(mm / 25.4, 3) for k, mm in p.items()}
    # pioggia piezo (come la manda un gateway Ecowitt con WS90)...
    campi.update({
        "rrain_piezo": pollici["rate"], "erain_piezo": pollici["event"],
        "hrain_piezo": pollici["hourly"], "drain_piezo": pollici["daily"],
        "wrain_piezo": pollici["weekly"], "mrain_piezo": pollici["monthly"],
        "yrain_piezo": pollici["yearly"],
    })
    # ...e anche nei campi "classici", per i programmi che leggono solo quelli
    campi.update({
        "rainratein": pollici["rate"], "eventrainin": pollici["event"],
        "hourlyrainin": pollici["hourly"], "dailyrainin": pollici["daily"],
        "weeklyrainin": pollici["weekly"], "monthlyrainin": pollici["monthly"],
        "yearlyrainin": pollici["yearly"], "totalrainin": pollici["total"],
    })
    return campi


def invia(campi: dict) -> str:
    corpo = urllib.parse.urlencode(campi).encode()
    req = urllib.request.Request(
        URL_WEBCAM_MANAGER, data=corpo, method="POST",
        headers={"Content-Type": "application/x-www-form-urlencoded"})
    with urllib.request.urlopen(req, timeout=15) as r:
        return f"HTTP {r.status}"


def riassunto(stato: Stato) -> str:
    v = stato.valori
    vel = v.get("velocita") or []
    parti = []
    if "temperatura" in v:
        parti.append(f"{v['temperatura']:.1f}°C")
    if "umidita" in v:
        parti.append(f"{v['umidita']:.0f}%")
    if "pressione" in v:
        parti.append(f"{v['pressione']:.1f} hPa")
    if vel:
        parti.append(f"vento {min(vel):.1f} m/s raffica {max(vel):.1f} m/s")
    if "direzione" in v:
        parti.append(f"dir {v['direzione']:.0f}°")
    parti.append(f"pioggia oggi {stato.pioggia(time.time())['daily']:.1f} mm")
    return ", ".join(parti)


# ---------------------------------------------------------------------------
# Programma principale
# ---------------------------------------------------------------------------
def configura_log():
    log.setLevel(logging.INFO)
    fmt = logging.Formatter("%(asctime)s %(levelname)s %(message)s", "%Y-%m-%d %H:%M:%S")
    console = logging.StreamHandler(sys.stdout)
    console.setFormatter(fmt)
    log.addHandler(console)
    try:
        file = logging.handlers.RotatingFileHandler(
            FILE_LOG, maxBytes=1_000_000, backupCount=3, encoding="utf-8")
        file.setFormatter(fmt)
        log.addHandler(file)
    except Exception:  # noqa: BLE001
        pass


async def main(prova: bool):
    from bleak import BleakScanner   # importato qui per poter testare il resto senza bleak

    stato = Stato()
    mac_cercato = MAC_WS90.upper()

    def callback(device, adv):
        dati = adv.service_data.get(BTHOME_UUID)
        if dati is None:
            return
        nome = adv.local_name or device.name or ""
        if mac_cercato:
            if device.address.upper() != mac_cercato:
                return
        elif not nome.upper().startswith("SBWS-90"):
            return
        try:
            if stato.aggiorna(decodifica_bthome(dati), time.time()):
                log.debug("pacchetto %s", dati.hex())
        except Exception as e:  # noqa: BLE001
            log.warning("Pacchetto non decodificabile (%s): %s", e, dati.hex())

    log.info("WS90 bridge %s avviato. Destinazione: %s%s", VERSIONE, URL_WEBCAM_MANAGER,
             "  [MODALITA' PROVA: non invio nulla]" if prova else "")
    log.info("Cerco il WS90 %s ...", mac_cercato or "(nome SBWS-90*)")

    scanner = None
    prossimo_invio = time.time() + 20   # primo invio dopo 20 s, per raccogliere i dati
    while True:
        try:
            if scanner is None:
                scanner = BleakScanner(detection_callback=callback)
                await scanner.start()
            await asyncio.sleep(1)
        except Exception as e:  # noqa: BLE001
            log.error("Errore Bluetooth: %s. Riprovo tra 30 secondi.", e)
            try:
                if scanner is not None:
                    await scanner.stop()
            except Exception:  # noqa: BLE001
                pass
            scanner = None
            await asyncio.sleep(30)
            continue

        adesso = time.time()
        if adesso < prossimo_invio:
            continue
        prossimo_invio = adesso + INTERVALLO_INVIO_SECONDI

        if not stato.ultimo_ricevuto:
            log.warning("Nessun dato ancora ricevuto dal WS90.")
            continue
        eta = adesso - stato.ultimo_ricevuto
        if eta > DATI_VECCHI_SECONDI:
            log.warning("Nessun dato dal WS90 da %d minuti: non invio. "
                        "Controlla distanza e pile.", eta // 60)
            # se il Bluetooth si e' bloccato, lo riavviamo
            try:
                await scanner.stop()
            except Exception:  # noqa: BLE001
                pass
            scanner = None
            continue

        campi = componi_ecowitt(stato, adesso)
        stato.salva()
        if prova:
            log.info("PROVA - %s", riassunto(stato))
            log.info("PROVA - invierei: %s", urllib.parse.urlencode(campi))
            continue
        try:
            esito = await asyncio.to_thread(invia, campi)
            log.info("Inviato (%s): %s", esito, riassunto(stato))
        except urllib.error.HTTPError as e:
            log.error("Webcam Manager ha risposto con errore HTTP %s", e.code)
        except Exception as e:  # noqa: BLE001
            log.error("Invio a Webcam Manager non riuscito: %s", e)


if __name__ == "__main__":
    configura_log()
    try:
        asyncio.run(main(prova="--prova" in sys.argv))
    except KeyboardInterrupt:
        log.info("Chiuso.")
