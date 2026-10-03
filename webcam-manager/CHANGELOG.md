# Changelog

Ogni versione rilasciata ha qui la sua voce: il testo diventa le note della
release mostrate nel pannello (Impostazioni → Aggiornamenti).

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
