# Distinta base (BOM) — parti standard da comprare

Componenti generici, reperibili da qualunque rivenditore di elettronica/
stampa 3D. Nessun link incluso: cerca per le specifiche indicate, i prezzi
variano molto per fornitore.

## Cosa NON serve più comprare

Rispetto a una prima versione, quasi tutta la minuteria meccanica è stata
sostituita da incastri ed elementi stampati (vedi `3d-print/README.md`,
sezione "Niente più ferramenta"): niente più angolari/viti agli spigoli
del mobile, niente M4+dadi+listello alle giunzioni dei pannelli spezzati,
niente dadi/inserti per housing/tramoggia/pannello frontale (vite
stampata in una bocchetta filettata stampata), niente cerniera comprata
per il coperchio tramoggia (nocche stampate + perno di filamento), niente
piedini di gomma (stampati integrati). **Le uniche viti comprate rimaste
sono le 4 del motore** (filettate nel suo corpo metallico, non c'è
alternativa stampata) — tutto il resto qui sotto è quello che resta
davvero da comprare.

## Meccanica / motion

| Componente | Specifica | Qtà | Note |
|---|---|---|---|
| Motore passo-passo | NEMA17, 1.0-1.5A, 34-40mm corpo | 1 | Il rullo ha attrito/carico bassi: non serve un NEMA17 "pesante". Si avvita da FUORI sul pannello laterale "drive" del mobile (foro pattern 31mm già nel pannello) — resta fuori dal mobile, solo l'albero entra. **Misura il diametro del suo alberino** (tipicamente 5mm) e confrontalo con `motor_shaft_d` in `3d-print/params.scad` prima di stampare l'accoppiatore |
| Driver motore | A4988 o DRV8825 | 1 | Pilotato da ESP32 via ESPHome |
| Alimentatore driver | 12V 1.5-2A (separato dalla logica) | 1 | Alimenta solo il motore |
| Cuscinetto 688ZZ (opzionale) | 8×16×5mm | 1 | Sul perno folle dell'housing, per durata; una boccola stampata basta per uso domestico |
| Pulsante momentaneo antivandalo | Ø16mm, IP65, illuminato o no | 1 | Pulsante frontale di erogazione — vedi `3d-print/front_panel.scad` |
| Viti M3×20 | — | 4 | **Le uniche viti comprate**: fissano il motore al suo pattern NEMA17 (fori filettati nel corpo motore, non ci si avvita una vite stampata) |
| Colla (cianoacrilica gel o epossidica bicomponente) | ~20-30ml | 1 | Incolla le giunzioni permanenti del mobile (spigoli e giunzioni dei pannelli spezzati) — vedi `3d-print/README.md` |
| Filamento 1.75mm (lo stesso che stampi) | uno spezzone di ~6cm | — | Perno della cerniera stampata del coperchio tramoggia — non è una parte a sé, è un ritaglio del filamento che già usi |

**Giunto motore↔rullo**: nella cartella `3d-print` c'è ora `coupler.scad`,
un accoppiatore stampato (grano di bloccaggio sull'alberino motore + spina
trasversale lato rullo, riusa lo stesso foro che il rullo aveva già). È il
pezzo più sperimentale di questa revisione — non è stato possibile
stamparlo e provarlo sotto carico reale in questa sessione. Se dopo il
collaudo a vuoto (vedi il passo finale in `3d-print/README.md`) noti
slittamento, la soluzione di riserva è tornare a un giunto flessibile
comprato (5mm→6mm, reperibile in qualunque negozio di stampa 3D/CNC) —
in tal caso serve anche una vite di bloccaggio M3 o un grano M3, non
inclusi sopra perché non fanno parte del percorso di default.

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
stamparli. Gli spigoli e le giunzioni dei pannelli spezzati sono incastri
stampati (perni di centraggio, giunto a pettine) da incollare — niente più
staffe, viti M4 o listelli di rinforzo comprati; i 4 spigoli verticali
hanno in più una vite stampata di rinforzo (compresa nelle 20 **Q** di
`3d-print/screws.scad`, non ferramenta a parte). Dettagli assemblaggio in
`3d-print/README.md`.

| Componente | Specifica | Qtà | Note |
|---|---|---|---|
| Pannelli | stampa 3D (PETG/PLA, infill 15-20%), o legno multistrato 9mm/alluminio composito/plexiglass tagliati dalla stessa sagoma | 8 (sopra, sotto, 2 fianchi × 2 segmenti, retro × 2 segmenti) | Il fronte è `front_panel.scad`, non spezzato. Se tagli da pannello piatto invece di stampare: gli incastri (linguette/pettine) restano nel disegno ma sono pensati per la stampa — sulla giunzione dei segmenti valuta comunque un rinforzo incollato, sugli spigoli un angolare interno come nella versione precedente |
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
