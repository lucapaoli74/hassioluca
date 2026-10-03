# Webcam Manager

*made by Paoli Luca 2026 · [paoli.lu@gmail.com](mailto:paoli.lu@gmail.com)*

Programma per gestire le webcam di una località e pubblicarle sui siti.
È un **unico eseguibile** con **pannello web integrato**: niente da installare a parte lui (e ffmpeg per le telecamere RTSP, già incluso nel Setup per Windows).

- **Ricerca telecamere** in rete (ONVIF + scansione delle porte, riconosce Hikvision, EZVIZ, Dahua, Reolink, Axis, TP-Link…), con lettura automatica degli URL via ONVIF.
- **Vista live** di tutte le webcam (anche RTSP, convertite al volo per il browser) e delle telecamere appena trovate, prima ancora di aggiungerle.
- **Sovrimpressioni** a livelli, trascinabili con il mouse sull'anteprima: scritte, data/ora in italiano, loghi PNG, **dati live** (Open-Meteo, **Weather Underground** con ricerca delle stazioni vicine, Home Assistant, qualsiasi URL JSON) e **previsioni del tempo** a 1–5 giorni con icone.
- **Pubblicazione** via FTP, FTPS, SFTP, **OneDrive** o cartella locale, ciascuna con la sua **periodicità** e il nome file modificabile. Test di pubblicazione reale (carica, rinomina e cancella un file di prova).
- **Mini-sito storico** pubblicato insieme all'immagine: calendario navigabile, un'immagine ogni ora (o ogni 10/15/30 min), conservata per sempre sul sito, più il **timelapse delle ultime ore**.
- **Storico locale** con conservazione per periodo (1 mese … 5 anni) o spazio massimo per webcam.
- **Notifiche email** su errori di webcam o pubblicazione (e quando si risolvono).
- **Aggiornamenti automatici** da GitHub su tutte le installazioni, **backup automatico** della configurazione a ogni modifica, **esporta/importa** per replicare su altri PC, **diagnostica** da allegare alle segnalazioni.
- Pensato per **funzionare h24**: servizio con avvio automatico e riavvio in caso di errore, isolamento dei guasti, protezione dal disco pieno, log a rotazione, endpoint `/healthz` per il monitoraggio.

---

## Installazione

Tre modi, a scelta. In tutti i casi poi si lavora dal **pannello web**: `http://INDIRIZZO:8080` (utente `admin`).

> La macchina deve stare **nella stessa rete delle telecamere** (per una VM: scheda di rete in *bridge*, non NAT), altrimenti la ricerca automatica non le trova.

### 1. Windows (PC fisso, mini PC)

1. Scarica `WebcamManager-Setup-X.Y.Z.exe` dall'ultima [release](../../releases).
2. Avvialo: chiede **nome della località** (es. *Camping Coggiolo Sant'Anna Pelago (MO)*), **altitudine s.l.m.**, **password** e **porta** del pannello.
3. Alla fine si apre il pannello.

Il Setup configura Windows da solo:

- servizio **“Webcam Manager”** con avvio automatico (anche senza utenti collegati);
- riavvio automatico in caso di errore;
- regola nel firewall;
- sospensione disattivata quando il PC è collegato alla corrente.

I dati stanno in `C:\ProgramData\WebcamManager` (configurazione, loghi, storico, log).
Rilanciando un Setup più nuovo si aggiorna mantenendo tutto.

Da riga di comando (prompt amministratore) sono disponibili anche: `webcam-manager install | uninstall | start | stop | version`.

### 2. Proxmox VE

Sulla shell dell'host Proxmox:

```bash
curl -fsSLO https://raw.githubusercontent.com/lucapaoli74/hassioluca/main/webcam-manager/deploy/proxmox-create-vm.sh
bash proxmox-create-vm.sh --vmid 150 --bridge vmbr0 --storage local-lvm \
  --location "Camping Coggiolo Sant'Anna Pelago (MO)" --altitude 1250 --password "una-password"
```

Crea una VM Debian 12 (2 CPU, 2 GB RAM, 32 GB disco, avvio automatico con l'host) e al primo avvio installa da sola Webcam Manager e ffmpeg tramite cloud-init. Opzioni utili:

- `--ip 192.168.1.50/24 --gw 192.168.1.1` per un IP fisso;
- `--vlan 20` per una VLAN;
- `--disk 200G` se serve più spazio per lo storico.

Lo storage indicato con `--snippets` (default `local`) deve avere abilitato il contenuto *Snippets*: Datacenter → Storage.

### 3. VMware vCenter / ESXi

1. Crea una VM **Debian 12** o **Ubuntu 22.04/24.04** (2 vCPU, 2 GB RAM, 32+ GB disco) con la scheda di rete sul **port group della LAN delle telecamere**.
2. Nella VM, come root:
   ```bash
   curl -fsSL https://raw.githubusercontent.com/lucapaoli74/hassioluca/main/webcam-manager/deploy/install-linux.sh | bash -s -- \
     --location "Camping Coggiolo Sant'Anna Pelago (MO)" --altitude 1250 --password "una-password"
   ```

**In automatico con cloud-init**: usa l'immagine cloud Ubuntu (OVA) e passa [`deploy/cloud-init.yaml`](deploy/cloud-init.yaml), dopo aver modificato i valori, come *user-data*. Lo puoi fare in due modi:

- da vSphere: proprietà della vApp → *User Data*, codificato in base64;
- con `govc`:
  ```bash
  govc vm.change -vm webcam-manager \
    -e guestinfo.userdata="$(base64 -w0 cloud-init.yaml)" -e guestinfo.userdata.encoding=base64
  ```

Lo stesso script `install-linux.sh` funziona su qualsiasi Linux Debian/Ubuntu, anche su Raspberry Pi. Rilanciandolo si aggiorna il programma.

**Repository privato**: gli script si scaricano da GitHub. Se il repository è privato servono due accorgimenti:

- imposta `GITHUB_TOKEN` (token in sola lettura);
- copia `install-linux.sh` sulla VM e usa `--binary percorso/eseguibile`.

### Altri sistemi

C'è anche un `Dockerfile` per NAS e server Linux. Va usato con `--network host`, altrimenti la ricerca delle telecamere non funziona. Su Windows conviene il Setup: Docker Desktop isola la rete e la ricerca non trova le telecamere.

---

## Primi passi

1. **Ricerca telecamere** → *Avvia ricerca*. Inserisci le credenziali e, per le ONVIF, premi *Leggi URL via ONVIF*.
   Per ogni URL ci sono due pulsanti:
   - **Live**, per vederla subito;
   - **Aggiungi / pubblica**, per configurarla.

   **EZVIZ**: l'utente è `admin` e la password è il *codice di verifica* (6 lettere sull'etichetta, visibile anche nell'app EZVIZ). Se la crittografia delle immagini è disattivata, prova anche senza password. Se non risponde, attiva RTSP / “Visione live in LAN” nell'app.
2. **Webcam**: nome, intervallo di cattura, fascia oraria, storico (frequenza e conservazione), timelapse, siti su cui pubblicarla.
3. **Sovrimpressioni**: aggiungi livelli e trascinali sull'anteprima. Segnaposto disponibili:
   - `{location}` `{altitude}` `{name}` `{date}` `{time}` `{longdate}` `{weekday}`;
   - `{data:ID}` per i dati live;
   - `{site_url}` `{history_url}` per l'indirizzo del sito.
4. **Dati live** → *Cerca stazioni Weather Underground*: scegli la stazione, spunta i valori e crea la scritta in un clic. In alternativa Open-Meteo, gratis e senza chiave.
5. **Siti FTP**: server, cartella, webcam e periodicità, poi **Test pubblicazione**. Con *mini-sito storico* attivo, lo storico è su `…/cartella/storico/`.
6. **Impostazioni**:
   - *Notifiche email*: SMTP e destinatari, con email di prova;
   - *Aggiornamenti*;
   - *Backup e replica*;
   - *Diagnostica*.

Le coordinate GPS si possono scegliere in tre modi:

- incollando un **link di Google Maps** (o le coordinate copiate col tasto destro sul punto);
- dalla **mappa** integrata;
- e si verificano con *Verifica su Google Maps*.

### OneDrive

Serve una volta sola un'app registrata su Azure:

1. Vai su <https://portal.azure.com> → *Registrazioni app* → *Nuova registrazione*.
2. Come tipi di account scegli *account personali Microsoft*, oppure anche quelli aziendali.
3. In *Autenticazione* attiva **Consenti flussi client pubblici**.
4. In *Autorizzazioni API* aggiungi Microsoft Graph → `Files.ReadWrite` e `offline_access`.
5. Copia l'**ID applicazione (client)** nel sito di tipo OneDrive, salva e premi **Collega account Microsoft**: compare un codice da inserire su microsoft.com/devicelogin.

In alternativa usa il tipo *cartella locale* puntando alla cartella OneDrive sincronizzata del PC. Il servizio però gira come utente di sistema, quindi va bene solo se OneDrive è sincronizzato per quell'utente.

---

## Aggiornamenti e gestione delle modifiche

- **Versioni**: ogni modifica rilasciata ha un numero (`1.2.0`) e una voce in [CHANGELOG.md](CHANGELOG.md).
- **Rilascio**: si aggiunge la voce al CHANGELOG, poi `git tag webcam-manager-v1.2.0 && git push --tags`. GitHub Actions esegue i test, compila tutte le piattaforme, crea il Setup.exe e `latest.json` e pubblica la release.
- **Le installazioni si aggiornano da sole**. In *Impostazioni → Aggiornamenti* scegli la fonte:
  - *GitHub*, con il repository `lucapaoli74/hassioluca` e un token in sola lettura se è privato;
  - *URL di latest.json* su un tuo sito.

  Ogni PC controlla ogni N ore, verifica l'impronta SHA-256, sostituisce l'eseguibile e il servizio riparte. Configurazione e storico non vengono toccati. Se un aggiornamento cambia il formato della configurazione, la converte da sé dopo una copia di sicurezza.
- **Versione di prova a un solo PC**: *Aggiornamenti → Aggiornamento manuale*, caricando l'eseguibile.
- **Annullare una modifica di configurazione**: *Backup e replica → Copie di sicurezza* (una per ogni salvataggio, ultime 50).
- **Segnalare problemi o chiedere modifiche**: GitHub → *Issues* → *New issue* con i modelli “Webcam Manager: problema” e “richiesta di modifica”. Allega la diagnostica (*Impostazioni → Diagnostica*), che non contiene password.

## Monitoraggio

`GET /healthz` (senza password) risponde `200` se tutto funziona e `503` se ci sono errori in corso. Va bene per Uptime Kuma, PRTG, Zabbix…

---

## Per lo sviluppo

Serve Go 1.26 o più recente (lo scarica da sé se manca).

```bash
go test ./...            # test, inclusa una prova completa cattura → pubblicazione
go run . -data ./data    # avvia in locale; la password iniziale è nel log
./build.sh 1.2.0         # compila tutte le piattaforme in dist/
```

| File | Cosa contiene |
|------|---------------|
| `main.go` | avvio, comandi, servizio |
| `config.go` | formato della configurazione, validazione, backup, migrazioni |
| `capture.go` | acquisizione snapshot/MJPEG/RTSP, autenticazione Digest |
| `imageproc.go`, `forecast.go` | sovrimpressioni e previsioni |
| `scheduler.go` | cattura periodica, storico, pubblicazione |
| `publish.go`, `onedrive.go` | FTP/FTPS/SFTP/cartella/OneDrive |
| `archive.go`, `timelapse.go` | storico locale e timelapse |
| `discovery.go` | ricerca in rete e ONVIF |
| `datasources.go`, `wunderground.go` | dati live |
| `alerts.go`, `updater.go`, `robust.go` | email, aggiornamenti, robustezza |
| `server.go`, `web/` | pannello web e mini-sito storico |
| `installer/`, `deploy/` | Setup per Windows, VM Proxmox/vCenter |
