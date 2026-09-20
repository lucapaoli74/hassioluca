# Parti da stampare in 3D — meccanismo per sigarette sfuse

File parametrici OpenSCAD (testati con `openscad 2021.01`, ogni pezzo
esporta una mesh manifold/chiusa — verificato in sessione con
`openscad -o out.stl <file>.scad`; l'assemblaggio completo è stato
renderizzato senza errori).

## Come funziona: rullo scanalato (singolarizzatore)

Non è più un carosello di pacchetti: la macchina tiene le sigarette
**sfuse alla rinfusa** in una tramoggia (>=100 pezzi) e le eroga **una alla
volta** tramite un rullo scanalato, lo stesso principio dei contapillole
farmaceutici e delle seminatrici a rullo (una tecnologia collaudata, non
inventata qui):

1. Il **rullo** (`roller.scad`) ha `flutes` scanalature cilindriche lungo
   la sua superficie, ciascuna dimensionata per contenere **una sola**
   sigaretta (troppo corta e stretta perché ce ne stiano due, per
   costruzione — vedi i commenti in `params.scad`).
2. Il rullo gira parzialmente immerso nella **tramoggia** (`hopper.scad`),
   nella zona di carico: le sigarette cadono per gravità nelle scanalature
   che passano di lì.
3. Una **griglia di sicurezza** (`grille.scad`) siede tra tramoggia e
   rullo: maglie abbastanza strette da bloccare un dito, abbastanza larghe
   da far cadere le sigarette.
4. Un **alloggiamento a "C"** (`housing.scad`) avvolge il resto della
   circonferenza, trattenendo le sigarette già raccolte durante il
   trasporto e facendo da "pettine": qualunque sigaretta non seduta bene
   nella scanalatura (di traverso, o doppia) viene bloccata dal bordo
   dell'apertura e ricade nella tramoggia.
5. Quando la scanalatura carica raggiunge l'apertura di scarico (in
   basso), la sigaretta cade per gravità nello **scivolo a labirinto**
   (`chute.scad` — alette sfalsate anti-intrusione) e arriva al
   **pannello frontale** (`front_panel.scad`), dove l'utente la preleva
   dalla feritoia.
6. Il motore passo-passo ruota il rullo esattamente di `360°/flutes` per
   ogni erogazione: una pressione sul pulsante frontale = una sigaretta
   (vedi `../../software/esphome`) — **ma solo se il firmware conferma che
   lo sportello della tramoggia è chiuso**, vedi sotto.
7. Tutto questo va dentro un **mobile chiuso** (`cabinet.scad`, pannelli
   piatti da taglio) — il meccanismo da solo non è sicuro da lasciare a
   vista.

## File

| File | Cosa produce | Copie | Tecnica |
|---|---|---|---|
| `roller.scad` | Rullo scanalato | 1 | stampa 3D |
| `housing.scad` | Alloggiamento a "C" + piatti terminali | 1 | stampa 3D |
| `grille.scad` | Griglia di sicurezza tramoggia→rullo | 1 | stampa 3D |
| `hopper.scad` | Tramoggia (>=100 sigarette) + bordo per coperchio | 1 | stampa 3D |
| `hopper_lid.scad` | Coperchio della tramoggia (cerniera + serratura) | 1 | stampa 3D |
| `chute.scad` | Scivolo a labirinto — **da adattare alla profondità del tuo mobile** | 1 | stampa 3D |
| `front_panel.scad` | Pannello con pulsante, LED, feritoia | 1 | stampa 3D o dima per taglio |
| `cabinet.scad` | Pannelli del mobile (sopra/sotto/fianchi/retro) | 5 | taglio laser/CNC/sega (esporta `.dxf`) |
| `params.scad` | Parametri condivisi — **modifica solo questo file** | — | — |
| `helpers.scad` | Funzioni geometriche condivise | — | — |
| `assembly.scad` | Solo anteprima in OpenSCAD, non da stampare | — | — |

## Sicurezza: come chiudere la macchina

Il rullo in movimento e l'apertura della tramoggia sono un rischio di
intrappolamento dita se lasciati accessibili. Il progetto lo affronta su
più livelli (dettagli e limiti onesti in `../bom.md`, sezione "Nota sulla
sicurezza meccanica"):

- **Griglia** fissa tra tramoggia e rullo (`grille.scad`)
- **Scivolo a labirinto** tra rullo e feritoia frontale (`chute.scad`)
- **Sportello con serratura elettrica + interblocco firmware**:
  `hopper_lid.scad` si chiude a chiave sulla tramoggia; un sensore
  magnetico conferma al firmware che è chiuso, e **il motore non parte se
  non lo è** — qualunque sia l'origine del comando (pulsante, app,
  dashboard). Vedi `../../software/esphome/cigarette-dispenser.yaml`.
- **Mobile chiuso** (`cabinet.scad`): l'unica cosa accessibile dall'esterno
  deve restare il pulsante, la feritoia e lo sportello a chiave.

Nessuna di queste è una certificazione formale (non ISO 13857/EN60204) —
sono accorgimenti ragionevoli per un uso domestico consapevole, non per un
contesto con accesso libero di bambini piccoli.

## IMPORTANTE: misura le tue sigarette prima di stampare

`params.scad` assume un formato king-size standard con filtro
(~84×7.9mm). Le "JPS fini" che userai potrebbero differire di qualche
decimo di millimetro — misurale con un calibro e aggiorna `cig_l`, `cig_d`
in `params.scad` prima di stampare. Tutte le altre dimensioni (diametro
rullo, alloggiamento, tramoggia) sono derivate automaticamente.

Con i valori di default: rullo Ø 36.7mm × 88mm, alloggiamento Ø ~60mm,
tramoggia con capacità stimata 95-160 sigarette a seconda di quanto si
impaccano (vedi il calcolo commentato in `params.scad`) — abbondantemente
sopra le 100 richieste anche nello scenario pessimistico.

## Il singolarizzatore richiede una taratura empirica

Questo è vero per **qualsiasi** dispenser di oggetti alla rinfusa,
comprese le macchine commerciali: la geometria qui è corretta in
principio, ma la tenuta pratica (nessuna sigaretta doppia, nessun
inceppamento) dipende da attrito, umidità, velocità di rotazione e da
quanto la tramoggia è piena. Aspettati di dover:

- regolare `discharge_gap_deg`/`load_gap_deg` e il gioco `housing_clear`
  di qualche decimo di mm dopo i primi test
- rallentare la velocità di rotazione dello stepper se noti doppie
  erogazioni
- eventualmente aggiungere un piccolo agitatore/vibrazione nella tramoggia
  se le sigarette "fanno ponte" sopra il rullo (comune con oggetti
  cilindrici lunghi) — non incluso in questa prima versione

Il sensore di caduta (`../wiring.md`) è la tua rete di sicurezza software:
conferma che sia effettivamente caduta una sigaretta, e il firmware segnala
un errore se non lo è (vedi `../../software/esphome`).

## Impostazioni di stampa consigliate

- Materiale: **PETG** (l'attrito ripetuto rullo/sigaretta e sportello usura
  il PLA più in fretta)
- Altezza layer: 0.2 mm, 4 perimetri, infill 20%
- Nessun supporto necessario per `roller.scad`/`hopper.scad`/`chute.scad`
  se orientati come esportati; `housing.scad` potrebbe volere supporti
  minimi sotto gli sbalzi dei piatti terminali
- Foro albero del rullo (6mm): rifinisci con un trapano se stampa stretta

## Assemblaggio

1. Stampa 1× ciascuno di `roller.scad`, `housing.scad`, `grille.scad`,
   `hopper.scad`, `hopper_lid.scad`, `chute.scad`, `front_panel.scad`;
   taglia i 5 pannelli di `cabinet.scad`
2. Monta il rullo nell'housing, verifica che giri libero senza attrito
   eccessivo contro le pareti
3. Fissa il motore al piatto terminale "drive" (foro NEMA17, pattern
   31mm) con un giunto flessibile verso l'albero del rullo
4. Il perno opposto ("idler") scorre nel foro boccola del piatto
   terminale — se vuoi più durata, sostituiscilo con un piccolo
   cuscinetto (vedi `../bom.md`)
5. Assembla il mobile (pannelli + angolari, vedi `../bom.md`); il pannello
   superiore ha il ritaglio per la tramoggia
6. Incastra la griglia tra la bocca inferiore della tramoggia e
   l'apertura di carico dell'housing, poi fissa la tramoggia nel ritaglio
   del pannello superiore
7. Fissa lo scivolo sotto l'apertura di scarico, il pannello frontale in
   fondo allo scivolo (adatta lunghezza/percorso alla profondità del tuo
   mobile)
8. Monta il coperchio della tramoggia con la cerniera sul lato fronte,
   la serratura + il sensore magnetico sul lato retro (fori già presenti
   nel bordo stampato)
9. Monta pulsante, LED, sensore di caduta e sensore sportello secondo
   `../wiring.md`
10. **Prima di caricare sigarette vere**: fai girare il rullo a vuoto
    qualche ciclo (con lo sportello chiuso, altrimenti l'interblocco lo
    impedisce), verifica l'allineamento delle aperture, poi carica poche
    decine di sigarette per un primo collaudo prima di riempire la
    tramoggia del tutto
