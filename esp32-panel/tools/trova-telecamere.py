#!/usr/bin/env python3
"""Elenca le telecamere EZVIZ e Reolink della rete di casa e prepara la
configurazione per la pagina Telecamere del pannello.

Va lanciato su un PC collegato alla STESSA rete delle telecamere (non serve
installare nulla: solo Python 3):

    python3 trova-telecamere.py                         # solo ricerca
    python3 trova-telecamere.py --utente admin --password XXX \
        --codice-ezviz ABCDEF                           # ricerca + prova foto
    python3 trova-telecamere.py --rete 192.168.1.0/24   # rete indicata a mano

Cosa fa:
  1. ricerca automatica ONVIF (WS-Discovery) e SADP (protocollo di
     ricerca Hikvision, usato anche da EZVIZ);
  2. scansione delle porte tipiche (80, 443, 554 RTSP, 8000, 9000) su tutta
     la sottorete;
  3. riconosce le Reolink dalla loro API HTTP e le EZVIZ/Hikvision da
     SADP/ISAPI/porta 8000;
  4. con le credenziali: scarica una foto di prova da ogni telecamera
     (salvata come snapshot-<ip>.jpg) e prova i percorsi RTSP;
  5. stampa il blocco `telecamere:` da incollare in esphome/secrets.yaml e,
     per le EZVIZ, la configurazione di go2rtc.

Perché le EZVIZ passano da go2rtc: le loro foto istantanee via HTTP (ISAPI)
richiedono l'autenticazione "Digest", che il client HTTP di ESPHome non
gestisce, e spesso su EZVIZ l'HTTP locale è proprio chiuso. go2rtc
(https://github.com/AlexxIT/go2rtc, gratuito, un solo eseguibile + ffmpeg)
legge il flusso RTSP e offre la foto in JPEG su
http://<pc>:1984/api/frame.jpeg?src=<nome>. Funziona anche senza Home
Assistant. Su EZVIZ l'utente è `admin` e la password è il "codice di
verifica" stampato sull'etichetta della telecamera; l'RTSP va abilitato
dall'app EZVIZ (Impostazioni dispositivo → Rete locale / RTSP) su alcuni
modelli.
"""
import argparse
import concurrent.futures as cf
import hashlib
import ipaddress
import json
import re
import socket
import ssl
import struct
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid

PORTE = (80, 443, 554, 8000, 9000)
TIMEOUT = 0.6


# --------------------------------------------------------------------------
# Rete locale
# --------------------------------------------------------------------------
def rete_locale():
    s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    try:
        s.connect(("8.8.8.8", 80))  # nessun pacchetto inviato: solo routing
        ip = s.getsockname()[0]
    finally:
        s.close()
    return ip, ipaddress.ip_network(ip + "/24", strict=False)


def porta_aperta(ip, porta):
    try:
        with socket.create_connection((str(ip), porta), timeout=TIMEOUT):
            return True
    except OSError:
        return False


def scansione(rete):
    host = list(rete.hosts())
    risultati = {}
    with cf.ThreadPoolExecutor(max_workers=256) as ex:
        fut = {ex.submit(porta_aperta, ip, p): (str(ip), p)
               for ip in host for p in PORTE}
        for f in cf.as_completed(fut):
            ip, p = fut[f]
            if f.result():
                risultati.setdefault(ip, set()).add(p)
    return risultati


# --------------------------------------------------------------------------
# Ricerca multicast
# --------------------------------------------------------------------------
def multicast(msg, porta, attesa=3.0):
    s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM, socket.IPPROTO_UDP)
    s.setsockopt(socket.IPPROTO_IP, socket.IP_MULTICAST_TTL, 2)
    s.settimeout(0.3)
    risposte = []
    try:
        s.sendto(msg, ("239.255.255.250", porta))
        fine = time.time() + attesa
        while time.time() < fine:
            try:
                data, addr = s.recvfrom(65535)
                risposte.append((addr[0], data.decode("utf-8", "replace")))
            except socket.timeout:
                pass
    finally:
        s.close()
    return risposte


def ws_discovery():
    msg = f"""<?xml version="1.0" encoding="UTF-8"?>
<e:Envelope xmlns:e="http://www.w3.org/2003/05/soap-envelope"
 xmlns:w="http://schemas.xmlsoap.org/ws/2004/08/addressing"
 xmlns:d="http://schemas.xmlsoap.org/ws/2005/04/discovery"
 xmlns:dn="http://www.onvif.org/ver10/network/wsdl">
<e:Header><w:MessageID>uuid:{uuid.uuid4()}</w:MessageID>
<w:To>urn:schemas-xmlsoap-org:ws:2005:04:discovery</w:To>
<w:Action>http://schemas.xmlsoap.org/ws/2005/04/discovery/Probe</w:Action>
</e:Header><e:Body><d:Probe><d:Types>dn:NetworkVideoTransmitter</d:Types>
</d:Probe></e:Body></e:Envelope>""".encode()
    trovati = {}
    for ip, xml in multicast(msg, 3702):
        scopes = " ".join(re.findall(r"<[^>]*Scopes[^>]*>([^<]*)<", xml))
        info = {}
        for chiave in ("name", "hardware", "type"):
            m = re.search(rf"onvif://www\.onvif\.org/{chiave}/(\S+)", scopes)
            if m:
                info[chiave] = urllib.parse.unquote(m.group(1))
        trovati[ip] = info
    return trovati


def sadp():
    msg = (f'<?xml version="1.0" encoding="utf-8"?><Probe><Uuid>'
           f'{str(uuid.uuid4()).upper()}</Uuid><Types>inquiry</Types></Probe>'
           ).encode()
    trovati = {}
    for ip, xml in multicast(msg, 37020):
        def tag(t):
            m = re.search(rf"<{t}>([^<]*)</{t}>", xml)
            return m.group(1) if m else ""
        ip_dev = tag("IPv4Address") or ip
        trovati[ip_dev] = {"modello": tag("DeviceDescription") or tag("DeviceType"),
                           "seriale": tag("DeviceSN"), "mac": tag("MAC")}
    return trovati


# --------------------------------------------------------------------------
# Riconoscimento via HTTP
# --------------------------------------------------------------------------
_CTX = ssl.create_default_context()
_CTX.check_hostname = False
_CTX.verify_mode = ssl.CERT_NONE  # le telecamere usano certificati autofirmati


def http_get(url, timeout=4, auth=None):
    """GET con gestione opzionale di Basic/Digest (auth=(utente, password))."""
    req = urllib.request.Request(url, headers={"User-Agent": "pannello-esp32"})
    handlers = [urllib.request.HTTPSHandler(context=_CTX)]
    if auth:
        pm = urllib.request.HTTPPasswordMgrWithDefaultRealm()
        pm.add_password(None, url, auth[0], auth[1])
        handlers += [urllib.request.HTTPDigestAuthHandler(pm),
                     urllib.request.HTTPBasicAuthHandler(pm)]
    opener = urllib.request.build_opener(*handlers)
    try:
        with opener.open(req, timeout=timeout) as r:
            return r.status, dict(r.headers), r.read()
    except urllib.error.HTTPError as e:
        return e.code, dict(e.headers or {}), b""
    except (OSError, ValueError):
        return None, {}, b""


def schema(porte):
    return "http" if 80 in porte else "https"


def e_reolink(ip, porte):
    if 80 not in porte and 443 not in porte:
        return False
    st, _, body = http_get(f"{schema(porte)}://{ip}/cgi-bin/api.cgi?cmd=GetDevInfo")
    if st is None:
        return False
    try:
        data = json.loads(body.decode("utf-8", "replace"))
        return isinstance(data, list) and data and "cmd" in data[0]
    except ValueError:
        return False


def e_isapi(ip, porte):
    if 80 not in porte and 443 not in porte:
        return False
    st, h, _ = http_get(f"{schema(porte)}://{ip}/ISAPI/System/deviceInfo")
    auth = " ".join(v for k, v in h.items() if k.lower() == "www-authenticate")
    return st == 401 and ("Digest" in auth or "Basic" in auth)


# --------------------------------------------------------------------------
# Prove con credenziali
# --------------------------------------------------------------------------
def info_jpeg(data):
    """(larghezza, altezza, progressivo) oppure None se non è un JPEG."""
    if not data.startswith(b"\xff\xd8"):
        return None
    i = 2
    while i + 9 < len(data):
        if data[i] != 0xFF:
            i += 1
            continue
        marker = data[i + 1]
        if marker in (0xC0, 0xC1, 0xC2):
            h, w = struct.unpack(">HH", data[i + 5:i + 9])
            return w, h, marker == 0xC2
        if marker in (0xD8, 0x01) or 0xD0 <= marker <= 0xD7:
            i += 2
            continue
        seglen = struct.unpack(">H", data[i + 2:i + 4])[0]
        i += 2 + seglen
    return None


def prova_foto(ip, url, auth=None):
    st, _, data = http_get(url, timeout=8, auth=auth)
    info = info_jpeg(data) if st == 200 else None
    if not info:
        return None
    with open(f"snapshot-{ip}.jpg", "wb") as f:
        f.write(data)
    return info


def rtsp_describe(ip, percorso, utente, password, porta=554):
    """True se il percorso RTSP esiste ed è accessibile (gestisce Digest)."""
    url = f"rtsp://{ip}:{porta}{percorso}"

    def invia(s, cseq, extra=""):
        req = (f"DESCRIBE {url} RTSP/1.0\r\nCSeq: {cseq}\r\n"
               f"Accept: application/sdp\r\n{extra}\r\n")
        s.sendall(req.encode())
        return s.recv(4096).decode("utf-8", "replace")

    try:
        with socket.create_connection((ip, porta), timeout=3) as s:
            s.settimeout(3)
            r = invia(s, 1)
            if r.startswith("RTSP/1.0 200"):
                return True
            m = re.search(r'WWW-Authenticate:\s*Digest\s+(.*)', r, re.I)
            if m:
                p = dict(re.findall(r'(\w+)="([^"]*)"', m.group(1)))
                ha1 = hashlib.md5(f"{utente}:{p['realm']}:{password}".encode()).hexdigest()
                ha2 = hashlib.md5(f"DESCRIBE:{url}".encode()).hexdigest()
                resp = hashlib.md5(f"{ha1}:{p['nonce']}:{ha2}".encode()).hexdigest()
                hdr = (f'Authorization: Digest username="{utente}", realm="{p["realm"]}", '
                       f'nonce="{p["nonce"]}", uri="{url}", response="{resp}"\r\n')
            elif re.search(r"WWW-Authenticate:\s*Basic", r, re.I):
                import base64
                tok = base64.b64encode(f"{utente}:{password}".encode()).decode()
                hdr = f"Authorization: Basic {tok}\r\n"
            else:
                return False
            return invia(s, 2, hdr).startswith("RTSP/1.0 200")
    except (OSError, KeyError):
        return False


# percorsi RTSP: prima il flusso secondario (risoluzione bassa, più leggero)
RTSP_EZVIZ = ("/Streaming/Channels/102", "/h264/ch1/sub/av_stream",
              "/Streaming/Channels/101", "/H.264", "/h264_stream")
RTSP_REOLINK = ("/h264Preview_01_sub", "/Preview_01_sub", "/h264Preview_01_main")


def nome_pulito(testo):
    return re.sub(r"[^a-z0-9]+", "_", testo.lower()).strip("_") or "cam"


# --------------------------------------------------------------------------
def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--rete", help="es. 192.168.1.0/24 (default: la /24 di questo PC)")
    ap.add_argument("--utente", default="admin", help="utente Reolink (default admin)")
    ap.add_argument("--password", help="password Reolink")
    ap.add_argument("--codice-ezviz", help="codice di verifica EZVIZ (etichetta)")
    ap.add_argument("--go2rtc", help="IP del PC su cui girerà go2rtc (default: questo PC)")
    a = ap.parse_args()

    mio_ip, rete = rete_locale()
    if a.rete:
        rete = ipaddress.ip_network(a.rete, strict=False)
    go2rtc_ip = a.go2rtc or mio_ip
    print(f"Rete: {rete}  (questo PC: {mio_ip})")

    print("Ricerca ONVIF e SADP...", flush=True)
    with cf.ThreadPoolExecutor(2) as ex:
        f_onvif, f_sadp = ex.submit(ws_discovery), ex.submit(sadp)
        onvif, hik = f_onvif.result(), f_sadp.result()

    print(f"Scansione porte {', '.join(map(str, PORTE))} su {rete.num_addresses} indirizzi...",
          flush=True)
    porte = scansione(rete)

    candidati = set(onvif) | set(hik) | {ip for ip, p in porte.items()
                                          if p & {554, 8000, 9000}}
    if not candidati:
        print("\nNessuna telecamera trovata. Il PC è sulla stessa rete/VLAN delle "
              "telecamere? Prova --rete 192.168.X.0/24.")
        return

    telecamere = []
    for ip in sorted(candidati, key=lambda x: ipaddress.ip_address(x)):
        p = porte.get(ip, set())
        o, h = onvif.get(ip, {}), hik.get(ip, {})
        if e_reolink(ip, p) or 9000 in p:
            marca = "Reolink"
        elif h or 8000 in p or e_isapi(ip, p):
            marca = "EZVIZ/Hikvision"
        else:
            marca = "sconosciuta"
        modello = h.get("modello") or o.get("hardware") or ""
        nome = o.get("name") or modello or ip
        telecamere.append({"ip": ip, "porte": sorted(p), "marca": marca,
                           "modello": modello, "nome": nome})

    print(f"\n{'IP':<16}{'Marca':<17}{'Modello':<22}Porte aperte")
    for t in telecamere:
        print(f"{t['ip']:<16}{t['marca']:<17}{t['modello'][:21]:<22}"
              f"{', '.join(map(str, t['porte']))}")

    if not (a.password or a.codice_ezviz):
        print("\nPer provare le foto e generare la configurazione rilancia con "
              "--password (Reolink) e/o --codice-ezviz (EZVIZ).")
        return

    righe, go2rtc = [], []
    print()
    for t in telecamere:
        ip, nome = t["ip"], t["nome"]
        if t["marca"] == "Reolink" and a.password:
            q = urllib.parse.urlencode({"cmd": "Snap", "channel": 0, "rs": "pannello",
                                        "user": a.utente, "password": a.password})
            base = f"{schema(t['porte'])}://{ip}/cgi-bin/api.cgi?{q}"
            for url in (base + "&width=640&height=360", base):
                info = prova_foto(ip, url)
                if info:
                    w, hh, prog = info
                    stato = "PROGRESSIVO: non leggibile dall'ESP32" if prog else "ok"
                    print(f"{ip}  Reolink foto {w}x{hh} {stato}  → snapshot-{ip}.jpg")
                    if not prog:
                        righe.append((f"Reolink {nome}", url))
                    break
            else:
                print(f"{ip}  Reolink: foto non riuscita (password? HTTP disattivato "
                      f"nelle impostazioni rete della telecamera?)")
        elif t["marca"] == "EZVIZ/Hikvision" and a.codice_ezviz:
            percorso = next((pp for pp in RTSP_EZVIZ
                             if rtsp_describe(ip, pp, "admin", a.codice_ezviz)), None)
            if percorso:
                src = nome_pulito(f"ezviz_{nome}")
                pw = urllib.parse.quote(a.codice_ezviz, safe="")
                go2rtc.append((src, f"rtsp://admin:{pw}@{ip}:554{percorso}"))
                righe.append((f"EZVIZ {nome}",
                              f"http://{go2rtc_ip}:1984/api/frame.jpeg?src={src}"))
                print(f"{ip}  EZVIZ RTSP ok: {percorso}  → via go2rtc come '{src}'")
            else:
                print(f"{ip}  EZVIZ: RTSP non accessibile (codice di verifica? RTSP "
                      f"disattivato nell'app? porta 554 aperta: {554 in t['porte']})")

    if righe:
        print("\n# ---- da incollare in esp32-panel/esphome/secrets.yaml ----")
        print("telecamere: |-\n  {")
        for n, u in righe:
            print(f'    "{n.replace(chr(34), "")}|{u}",')
        print("  }")
    if go2rtc:
        print(f"\n# ---- go2rtc.yaml (sul PC {go2rtc_ip}, serve anche ffmpeg) ----")
        print("streams:")
        for src, url in go2rtc:
            print(f"  {src}: {url}")
        print("api:\n  listen: \":1984\"")


if __name__ == "__main__":
    try:
        main()
    except KeyboardInterrupt:
        sys.exit(1)
