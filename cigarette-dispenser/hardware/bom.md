# Distinta base (BOM) — parti standard da comprare

Componenti generici, reperibili da qualunque rivenditore di elettronica/
stampa 3D (Amazon, AliExpress, negozi di componentistica per stampanti 3D
tipo Filoalfa/RS Components, o negozi locali). Nessun link incluso: cerca
per le specifiche indicate, i prezzi variano molto per fornitore.

## Meccanica / motion

| Componente | Specifica | Qtà | Note |
|---|---|---|---|
| Motore passo-passo | NEMA17, 1.5-1.8A, 40-48mm corpo | 1 | Indicizza il carosello |
| Driver motore | A4988 o DRV8825 | 1 | Pilotato da ESP32 via ESPHome |
| Alimentatore driver | 12V 2A (separato dalla logica) | 1 | Alimenta solo il motore |
| Giunto flessibile albero | 5mm (lato motore) → 8mm (lato mozzo) | 1 | Assorbe piccoli disallineamenti |
| Cuscinetto skate | 608ZZ (8×22×7mm) | 1 | Sede in `base.scad` |
| Viti M3×10 testa svasata | — | ~20 | Fissaggio spicchi/mozzo/coperchio |
| Viti M3×20 | — | 4 | Fissaggio motore |
| Dadi M3 / inserti termici M3 | — | ~15 | Se non stampi fori autofilettanti |
| Distanziali M3 | 15-20mm | 4-6 | Coperchio-base |

## Elettronica / controllo

| Componente | Specifica | Qtà | Note |
|---|---|---|---|
| ESP32 DevKit | ESP32-WROOM-32, 30/38 pin | 1 | Cervello del dispenser, gira ESPHome |
| Servo motore SG90 (o MG90S) | 9g o metal gear | 1 | Apre/chiude la paletta di uscita allo scivolo |
| Sensore fotointerruttore a forcella (slot sensor) | tipo TCST2103, o coppia IR emettitore/ricevitore | 1-2 | Conferma caduta pacchetto + homing carosello |
| Sensore fine corsa / hall + magnete | micro switch o A3144 + magnete | 1 | Riferimento "zero" del carosello (homing) |
| Lettore RFID/NFC | PN532 (I2C o UART) | 1 | Opzionale: sblocco solo per utenti autorizzati (vedi nota legale sotto) |
| Buzzer attivo 5V | — | 1 | Feedback erogazione/errore |
| LED di stato | RGB o singolo + resistenza | 1 | Stato macchina |
| Alimentatore logica | 5V 2A (USB o DC-DC da 12V) | 1 | Alimenta ESP32 + servo + sensori |
| Convertitore DC-DC 12V→5V | buck 3A | 1 | Se usi un solo alimentatore 12V per tutto |
| Cavi Dupont / JST | assortiti | — | Cablaggio |
| Serratura elettromagnetica o solenoide 12V | opzionale | 1 | Blocco sportello di carico anti-manomissione |

## Struttura macchina (non stampata)

| Componente | Specifica | Note |
|---|---|---|
| Mobile/cabinet | legno, metallo o profili in alluminio 2020 | Contiene il meccanismo, dimensioni secondo il tuo spazio |
| Cerniere + serratura a chiave | per lo sportello di servizio/rifornimento | Accesso solo al gestore |
| Vetro/plexiglass | per vetrina frontale, se prevista | Opzionale |

## Nota legale/etica importante

La vendita di sigarette tramite distributore automatico è regolata per
legge in quasi tutti i paesi (in Italia, ad esempio, i distributori
esterni non presidiati devono avere un sistema di verifica dell'età,
tipicamente lettura della tessera sanitaria/carta d'identità elettronica).
Questo progetto è pensato per **uso personale/domestico** (es. dispenser
auto-limitante integrato con Home Assistant per contingentare il proprio
consumo, con quota giornaliera impostabile — vedi
`../software/home-assistant`). Se hai intenzione di usarlo in un contesto
commerciale o accessibile a terzi, verifica la normativa locale su
distributori automatici di tabacco e verifica dell'età **prima** di
metterlo in funzione: potrebbe essere necessaria una licenza e un sistema
di verifica dell'età certificato, non solo il tag RFID opzionale elencato
sopra.
