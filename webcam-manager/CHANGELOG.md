# Changelog

Ogni versione rilasciata ha qui la sua voce: il testo diventa le note della
release mostrate nel pannello (Impostazioni → Aggiornamenti).

## [0.2.8] - 2026-10-17

- Privacy: opzione per webcam "pixela automaticamente i volti" (riconoscimento sul PC prima della pubblicazione, con sensibilità regolabile) e, facoltative, "Zone privacy" fisse sempre pixelate, da disegnare e spostare col mouse in Sovrimpressioni. Riconoscimento con la libreria pigo (licenza MIT, vedi privacy/LICENSE-pigo).

## [0.2.7] - 2026-10-15

- Telecamere RTSP: si cattura solo un fotogramma chiave completo e le immagini "colate" a strisce verticali (pacchetti persi, Wi-Fi debole) vengono scartate e ricatturate, così sul sito non va mai un'immagine rovinata.

## [0.2.6] - 2026-10-15

- Pagine meteo dinamiche (es. Parchi Emilia Centrale): Edge viene avviato in modo compatibile con il servizio di Windows (riprova senza sandbox e in modalità compatibile, con Chrome di riserva); il programma intercetta il file da cui la pagina prende i dati e lo legge direttamente; cerca gli indirizzi dei dati anche nei file JavaScript del sito.
- Ricerca dei valori più precisa: un numero vale solo se sta subito dopo l'etichetta (niente "1°" di "1° incontro").

## [0.2.5] - 2026-10-15

- Ricerca dei valori: riconosce "Temperatura 3,8" anche senza unità di misura accanto; campo "Parola vicina al valore" per prendere il numero che segue una parola; se non trova nulla mostra il testo letto nella pagina (con stato del browser) e il pulsante Copia per l'assistenza.

## [0.2.4] - 2026-10-15

- Pagine che caricano i dati con JavaScript: il valore si legge aprendo la pagina con il browser del computer (Microsoft Edge o Chrome) in modo invisibile, esattamente come la vede una persona.
- La ricerca dei valori scarta il rumore: niente percentuali da file grafici, niente "1°" di "1° incontro", niente pagine normali del sito.

## [0.2.3] - 2026-10-15

- "Cerca i valori nella pagina" funziona anche con le pagine che caricano i dati con JavaScript: cerca negli script, nei riquadri incorporati (widget delle stazioni meteo) e nei file di dati collegati (JSON, txt, xml…), mostra da dove arriva ogni valore e imposta da solo il tipo di lettura. Temperature in cima e nomi leggibili.

## [0.2.2] - 2026-10-15

- Nuovo dato live "Valore da una pagina web": si incolla l'indirizzo di una pagina (es. la pagina meteo di un rifugio), "Cerca i valori nella pagina" mostra temperature, umidità, vento… trovati e con un clic il valore scelto diventa un dato da mettere in sovrimpressione, riletto in automatico.

## [0.2.1] - 2026-10-15

- Modalità manutenzione per ogni webcam (pulsante 🛠️ in Stato e nella pagina della webcam): sui siti viene pubblicata l'ultima immagine buona scurita con un avviso al centro, testo modificabile. In manutenzione la telecamera non viene interrogata, lo storico non cresce e non partono email di errore.

## [0.2.0] - 2026-10-15

- Importa sovrimpressioni: pulsante nella pagina della webcam per clonare tutti i livelli da un'altra webcam.
- Le nuove webcam partono già con le sovrimpressioni dell'ultima webcam creata (si può scegliere da quale copiarle o usare quelle predefinite).
- Pagina Sovrimpressioni: "Importa da altra webcam" e "Applica a tutte le webcam".

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
