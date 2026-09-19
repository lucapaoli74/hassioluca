# Distinta base (BOM) — parti standard da comprare

Componenti generici, reperibili da qualunque rivenditore di elettronica/
stampa 3D. Nessun link incluso: cerca per le specifiche indicate, i prezzi
variano molto per fornitore.

## Meccanica / motion

| Componente | Specifica | Qtà | Note |
|---|---|---|---|
| Motore passo-passo | NEMA17, 1.0-1.5A, 34-40mm corpo | 1 | Il rullo ha attrito/carico bassi: non serve un NEMA17 "pesante" |
| Driver motore | A4988 o DRV8825 | 1 | Pilotato da ESP32 via ESPHome |
| Alimentatore driver | 12V 1.5-2A (separato dalla logica) | 1 | Alimenta solo il motore |
| Giunto flessibile albero | 5mm (lato motore) → 6mm (lato rullo) | 1 | Assorbe piccoli disallineamenti |
| Cuscinetto 688ZZ (opzionale) | 8×16×5mm | 1 | Sul perno folle dell'housing, per durata; una boccola stampata basta per uso domestico |
| Pulsante momentaneo antivandalo | Ø16mm, IP65, illuminato o no | 1 | Pulsante frontale di erogazione — vedi `3d-print/front_panel.scad` |
| Viti M3×10 testa svasata | — | ~30 | Fissaggio housing/piatti/pannello |
| Viti M3×20 | — | 4 | Fissaggio motore |
| Dadi M3 / inserti termici M3 | — | ~20 | Se non stampi fori autofilettanti |

## Elettronica / controllo

| Componente | Specifica | Qtà | Note |
|---|---|---|---|
| ESP32 DevKit | ESP32-WROOM-32, 30/38 pin | 1 | Cervello del dispenser, gira ESPHome |
| Fotointerruttore a forcella (slot sensor) | tipo TCST2103, o coppia IR emettitore/ricevitore | 1 | Conferma caduta di UNA sigaretta nello scivolo |
| Resistenza pull-up 10kΩ | — | 1 | Per il sensore su GPIO34 (input-only, niente pullup interno) |
| Buzzer attivo 5V | — | 1 | Feedback erogazione/quota raggiunta/errore |
| LED di stato | singolo colore + resistenza, o RGB | 1 | Stato macchina |
| Serratura elettromagnetica o solenoide 12V | opzionale | 1 | Blocca lo sportello di ricarica della tramoggia |
| Alimentatore logica | 5V 2A (USB o DC-DC da 12V) | 1 | Alimenta ESP32 + sensori + buzzer + LED |
| Convertitore DC-DC 12V→5V | buck 3A | 1 | Se usi un solo alimentatore 12V per tutto |
| Cavi Dupont / JST | assortiti | — | Cablaggio |

Nota: rispetto a una prima versione basata su pacchetti interi, questo
meccanismo **non usa più un servo** per una paletta di scarico — il rullo
scanalato rilascia la sigaretta per gravità solo quando la scanalatura
raggiunge l'apertura fissa di scarico, quindi non serve un attuatore
aggiuntivo lì.

## Struttura macchina (non stampata)

| Componente | Specifica | Note |
|---|---|---|
| Mobile/cabinet | legno, metallo o profili in alluminio 2020 | Contiene tramoggia, rullo e scivolo — profondità da adattare a `chute.scad` |
| Cerniera + serratura a chiave sulla tramoggia | per la ricarica | In alternativa/aggiunta alla serratura elettrica |
| Sigarette | JPS (o altro formato king-size ~84×7.9mm) | Misura le tue prima di stampare — vedi `3d-print/README.md` |

## Nota legale/etica importante

La vendita di sigarette tramite distributore automatico è regolata per
legge in quasi tutti i paesi (in Italia, ad esempio, i distributori
esterni non presidiati devono avere un sistema di verifica dell'età,
tipicamente lettura della tessera sanitaria/carta d'identità elettronica).
Questo progetto è pensato per **uso personale/domestico**: gestione remota
con quota giornaliera configurabile (es. 10/giorno) per contingentare il
proprio consumo, invece che un punto vendita per terzi. Se prevedi di
renderlo accessibile ad altre persone, verifica la normativa locale su
distributori automatici di tabacco e verifica dell'età prima di metterlo
in funzione: questo progetto, così com'è, non è un sistema di verifica
dell'età certificato.
