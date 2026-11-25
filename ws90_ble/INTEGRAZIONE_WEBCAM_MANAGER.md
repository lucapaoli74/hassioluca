# Integrare il WS90 (Bluetooth) dentro Webcam Manager

> **Fatto in Webcam Manager 0.4.5** (`webcam-manager/ws90.go`, `ble_scan*.go`): collegamento
> "Sensore WS90 via Bluetooth" in 🌦️ Stazione meteo con Trova WS90 / Associa / Disassocia.
> Resta da verificare sul PC del Poggiolo la ricezione Bluetooth quando gira come servizio.

Richiesta: aggiungere a Webcam Manager la ricezione diretta via Bluetooth del sensore
meteo **Ecowitt WS90 "Powered by Shelly"**, con due funzioni nell'interfaccia:

1. **Trova WS90** – cerca via Bluetooth i sensori WS90 vicini e li mostra (nome, indirizzo, segnale).
2. **Associa** – l'utente sceglie il sensore; Webcam Manager memorizza l'indirizzo e da quel
   momento ne riceve i dati da solo e li tratta come quelli di una stazione Ecowitt
   (come se arrivassero su `/data/report/`).

Webcam Manager gira come **servizio Windows** sul PC del camping Poggiolo (192.168.1.125, pannello porta 8080).

## Cosa sappiamo già (verificato il 25/11/2026 col sensore reale)

- Nome Bluetooth: `SBWS-90CM` (il nome può mancare in alcuni pacchetti: usare l'indirizzo).
- Indirizzo del sensore di prova: `E4:0A:75:B6:CE:BB`.
- Trasmette **advertising BLE** (non serve connettersi né fare pairing di Windows) nel formato
  aperto **BTHome v2, NON cifrato**, service data UUID `0000fcd2-0000-1000-8000-00805f9b34fb`
  (16-bit `0xFCD2`). Un pacchetto ogni ~8 secondi, alternando due tipi.
- Segnale a 1 m: circa -50 dBm.

### Formato BTHome v2
Byte 0 = info (bit 0 = cifrato, bit 5-7 = versione = 2). Poi coppie `id oggetto` + valore little-endian:

| id | grandezza | byte | segno | fattore | note |
|----|-----------|------|-------|---------|------|
| 0x00 | packet id | 1 | no | 1 | serve a scartare i doppioni |
| 0x01 | batteria % | 1 | no | 1 | |
| 0x04 | pressione hPa | 3 | no | 0.01 | assoluta |
| 0x05 | illuminamento lux | 3 | no | 0.01 | W/m² = lux / 126.7 (come Ecowitt) |
| 0x08 | punto di rugiada °C | 2 | sì | 0.01 | |
| 0x0C | tensione V | 2 | no | 0.001 | supercondensatore |
| 0x20 | pioggia in corso (0/1) | 1 | no | 1 | |
| 0x2E | umidità % | 1 | no | 1 | |
| 0x44 | velocità m/s | 2 | no | 0.01 | **compare due volte**: vento medio e raffica → usare min = medio, max = raffica |
| 0x45 | temperatura °C | 2 | sì | 0.1 | |
| 0x46 | indice UV | 1 | no | 0.1 | |
| 0x5E | direzione vento ° | 2 | no | 0.01 | |
| 0x5F | pioggia mm | 2 | no | 0.1 | **contatore cumulativo**: giornaliera/oraria/ecc. vanno calcolate dalle differenze, gestendo il ritorno a 0 (riavvio del sensore) e salvando lo stato su disco |

Pacchetti reali:
```
4000bf016404e68b0108cf030cc8002e3545c4005f0000   -> 19.6°C, 53%, 1013.5 hPa, rugiada 9.75, pioggia 0.0 mm
4000c005000000200044000044000046005e1860         -> lux 0, vento 0/0 m/s, UV 0, direzione 246°
```

## Implementazione di riferimento già funzionante
`ws90_ble/ws90_bridge.py` (Python + libreria `bleak`) in questo repository: decodifica, calcolo
pioggia (rate/evento/ora/giorno/settimana/mese/anno/totale), conversione nelle unità Ecowitt
(tempf, humidity, baromabsin/baromrelin, winddir, windspeedmph, windgustmph, solarradiation,
uv, *rain_piezo + campi rain classici) e invio ogni 60 s. Provato su Windows il 25/11/2026.
Da portare nel linguaggio di Webcam Manager.

In C#/.NET l'equivalente di `bleak` è `Windows.Devices.Bluetooth.Advertisement.BluetoothLEAdvertisementWatcher`
(leggere `Advertisement.DataSections` di tipo 0x16 = Service Data 16-bit, primi 2 byte `D2 FC`).

## Da verificare
- **Bluetooth da un servizio Windows**: il servizio gira in "session 0"; verificare sul PC del
  Poggiolo che il watcher BLE riceva i pacchetti anche da servizio. Se no, tenere un piccolo
  processo nell'utente (es. `ws90_bridge.py`) che inoltra a `/data/report/`.
- **Distanza** PC ↔ sensore: BLE ~10-30 m all'aperto. Eventualmente chiavetta BT USB con prolunga.
- **Pioggia**: non ancora verificata col sensore bagnato (contatore 0x5F).
