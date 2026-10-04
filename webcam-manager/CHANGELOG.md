# Changelog

Ogni versione rilasciata ha qui la sua voce: il testo diventa le note della
release mostrate nel pannello (Impostazioni → Aggiornamenti).

## [0.1.9] - 2026-10-04

- Corretto: creando un nuovo sito, webcam o dato live il campo Nome perdeva il fuoco a ogni lettera.

## [0.1.8] - 2026-10-04

- Pulsante "Trova altri flussi" nella pagina della webcam: prova i percorsi RTSP noti sulla stessa telecamera (utile per le telecamere a due obiettivi) e permette di vederli in live o aggiungerli come nuove webcam.

## [0.1.7] - 2026-10-04

- Installazione guidata con doppio clic: il programma chiede località, altitudine e password, ottiene i permessi di amministratore, si copia in Program Files e si installa come servizio. Se è già installato mostra lo stato e apre il pannello (o lo aggiorna con la nuova versione).

## [0.1.6] - 2026-10-04

- Se una telecamera non risponde i tentativi automatici si diradano fino a uno ogni 15 minuti (alcune telecamere bloccano chi riprova troppo spesso); "Cattura ora" riprova subito.

## [0.1.5] - 2026-10-04

- Ricerca telecamere molto più leggera (poche connessioni alla volta, meno porte): non viene più scambiata per un attacco dagli antivirus.

## [0.1.4] - 2026-10-04

- Diagnostica: se la porta RTSP non risponde controlla anche ping, porte 80/8000/443, test di Windows, programmi già collegati alla telecamera e antivirus installato.

## [0.1.3] - 2026-10-03

- Pulsante "Diagnostica" nella pagina della webcam: verifica porta, dialogo RTSP (credenziali, percorso, codec) e ffmpeg, con un rapporto da copiare senza password.

## [0.1.2] - 2026-10-03

- RTSP: regola del firewall di Windows anche per ffmpeg (necessaria in UDP), errore con l'esito di entrambi i tentativi TCP/UDP e messaggi di ffmpeg più dettagliati.

## [0.1.1] - 2026-10-03

- Telecamere RTSP: il tipo di sorgente si riconosce dall'indirizzo (rtsp://), prova TCP e poi UDP (necessario per alcune EZVIZ), messaggi d'errore più chiari senza password.
- ffmpeg viene scaricato automaticamente su Windows se manca; pulsante "Installa ffmpeg" in Impostazioni.
- Ricerca: aggiunto il percorso EZVIZ /h264_stream (modelli C1/C2/C3).

## [0.1.0] - 2026-10-03

Prima versione di prova.

- Pannello web con menu: Stato, Vista live, Ricerca telecamere, Webcam, Sovrimpressioni, Dati live, Siti FTP, Storico, Impostazioni.
- Ricerca telecamere ONVIF e per porte (Hikvision, EZVIZ, Dahua, Reolink, Axis, TP-Link…), lettura URL via ONVIF.
- Vista live (snapshot, MJPEG, RTSP tramite ffmpeg) anche delle telecamere non ancora aggiunte.
- Sovrimpressioni trascinabili: scritte, data/ora, loghi, dati live, previsioni del tempo, nome e altitudine della località.
- Dati live: Open-Meteo, Weather Underground (con ricerca stazioni), Home Assistant, URL JSON/testo.
- Pubblicazione FTP/FTPS/SFTP/OneDrive/cartella con periodicità per sito e test reale.
- Mini-sito storico navigabile e timelapse delle ultime ore; storico locale con conservazione per periodo o spazio.
- Notifiche email, aggiornamenti automatici, backup della configurazione, esporta/importa, diagnostica.
- Installazione: Setup per Windows (servizio), VM Proxmox e vCenter (cloud-init).
