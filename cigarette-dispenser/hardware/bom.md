# Distinta base (BOM) — parti standard da comprare

Componenti generici, reperibili da qualunque rivenditore di elettronica/
stampa 3D. Nessun link incluso: cerca per le specifiche indicate, i prezzi
variano molto per fornitore.

## Meccanica / motion

| Componente | Specifica | Qtà | Note |
|---|---|---|---|
| Motore passo-passo | NEMA17, 1.0-1.5A, 34-40mm corpo | 1 | Il rullo ha attrito/carico bassi: non serve un NEMA17 "pesante". Si avvita da FUORI sul pannello laterale "drive" del mobile (foro pattern 31mm già nel pannello) — resta fuori dal mobile, solo l'albero entra |
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
| Serratura elettromagnetica o solenoide 12V | tipo push-pull, corsa ≥10mm | 1 | **Non più opzionale**: blocca lo sportello tramoggia, vedi nota sicurezza sotto |
| Sensore magnetico (reed switch) | tipo per porte/finestre, NC o NA | 1 | Conferma software che lo sportello è chiuso prima di autorizzare il motore |
| Alimentatore logica | 5V 2A (USB o DC-DC da 12V) | 1 | Alimenta ESP32 + sensori + buzzer + LED |
| Convertitore DC-DC 12V→5V | buck 3A | 1 | Se usi un solo alimentatore 12V per tutto |
| Cavi Dupont / JST | assortiti | — | Cablaggio |

Nota: rispetto a una prima versione basata su pacchetti interi, questo
meccanismo **non usa più un servo** per una paletta di scarico — il rullo
scanalato rilascia la sigaretta per gravità solo quando la scanalatura
raggiunge l'apertura fissa di scarico, quindi non serve un attuatore
aggiuntivo lì.

## Struttura macchina — mobile chiuso

Il meccanismo va **sempre** racchiuso in un mobile: il rullo in movimento e
l'apertura della tramoggia non sono sicuri da toccare a corpo libero. Vedi
`3d-print/cabinet.scad` per i pannelli quotati — **ogni pezzo, spezzato dove
serve, entra nel piano di stampa di una Bambu X1C (256×256×256mm)**; lo
stesso file esporta anche `.dxf` se preferisci tagliarli invece di
stamparli. Dettagli assemblaggio in `3d-print/README.md`.

| Componente | Specifica | Qtà | Note |
|---|---|---|---|
| Pannelli | stampa 3D (PETG/PLA, infill 15-20%), o legno multistrato 9mm/alluminio composito/plexiglass tagliati dalla stessa sagoma | 8 (sopra, sotto, 2 fianchi × 2 segmenti, retro × 2 segmenti) | Il fronte è `front_panel.scad`, non spezzato |
| Viti M4×16 + dadi | per unire i segmenti spezzati lungo la giunzione | ~20 | 5 fori per giunzione, vedi `seam_hole_n` in `params.scad` |
| Listello/fascetta interna | legno o profilo stampato, a cavallo di ogni giunzione | 4 (una per ogni coppia di segmenti) | Irrigidisce la cucitura tra due metà dello stesso pannello |
| Angolari interni + viti | angolari in metallo o plastica, ~20×20mm | 8-12 | Uniscono i pannelli agli spigoli del mobile (giunto a battuta) |
| Cerniera piccola | 40-60mm, qualunque tipo (piano, a libro) | 1 | Per il coperchio della tramoggia (`hopper_lid.scad`), più affidabile di una cerniera stampata su uno sportello aperto spesso |
| Piedini in gomma | autoadesivi | 4 | Sul pannello inferiore |
| Sigarette | JPS (o altro formato king-size ~84×7.9mm) | — | Misura le tue prima di stampare — vedi `3d-print/README.md` |

## Nota sulla sicurezza meccanica

Il rullo scanalato è un punto di intrappolamento per le dita se lasciato
accessibile. Il progetto lo mitiga con tre livelli, nessuno dei quali è una
certificazione formale (non ISO 13857/EN60204) ma che insieme rendono la
macchina ragionevolmente sicura per un uso domestico consapevole:

1. **Griglia** (`3d-print/grille.scad`) tra tramoggia e rullo: maglie più
   strette di un dito, più larghe di una sigaretta.
2. **Sportello con serratura + interblocco elettrico**: il firmware
   rifiuta di muovere il motore se il sensore magnetico non conferma lo
   sportello chiuso (vedi `software/esphome/cigarette-dispenser.yaml`),
   qualunque sia l'origine del comando (pulsante, app, dashboard).
3. **Scivolo a labirinto** (`3d-print/chute.scad`): alette sfalsate tra la
   feritoia frontale e il rullo, per impedire di infilare una mano in
   linea retta fino al meccanismo.

Se costruisci questa macchina in un contesto con accesso di bambini
piccoli, non fidarti solo di queste misure: valuta un mobile con serratura
a chiave anche sul pannello frontale, o tieni la macchina fuori portata.

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
