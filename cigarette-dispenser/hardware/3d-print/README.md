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
3. Un **alloggiamento a "C"** (`housing.scad`) avvolge il resto della
   circonferenza, trattenendo le sigarette già raccolte durante il
   trasporto e facendo da "pettine": qualunque sigaretta non seduta bene
   nella scanalatura (di traverso, o doppia) viene bloccata dal bordo
   dell'apertura e ricade nella tramoggia.
4. Quando la scanalatura carica raggiunge l'apertura di scarico (in
   basso), la sigaretta cade per gravità nello **scivolo** (`chute.scad`)
   e arriva al **pannello frontale** (`front_panel.scad`), dove l'utente
   la preleva dalla feritoia.
5. Il motore passo-passo ruota il rullo esattamente di `360°/flutes` per
   ogni erogazione: una pressione sul pulsante frontale = una sigaretta
   (vedi `../../software/esphome`).

## File

| File | Cosa stampa | Copie |
|---|---|---|
| `roller.scad` | Rullo scanalato | 1 |
| `housing.scad` | Alloggiamento a "C" + piatti terminali | 1 |
| `hopper.scad` | Tramoggia (>=100 sigarette) | 1 |
| `chute.scad` | Scivolo verso il pannello — **da adattare alla profondità del tuo mobile** | 1 |
| `front_panel.scad` | Pannello con pulsante, LED, feritoia — o usalo solo come dima su legno/plexiglass | 1 |
| `params.scad` | Parametri condivisi — **modifica solo questo file** | — |
| `helpers.scad` | Funzioni geometriche condivise | — |
| `assembly.scad` | Solo anteprima in OpenSCAD, non da stampare | — |

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

1. Stampa 1× ciascuno di `roller.scad`, `housing.scad`, `hopper.scad`,
   `chute.scad`, `front_panel.scad`
2. Monta il rullo nell'housing, verifica che giri libero senza attrito
   eccessivo contro le pareti
3. Fissa il motore al piatto terminale "drive" (foro NEMA17, pattern
   31mm) con un giunto flessibile verso l'albero del rullo
4. Il perno opposto ("idler") scorre nel foro boccola del piatto
   terminale — se vuoi più durata, sostituiscilo con un piccolo
   cuscinetto (vedi `../bom.md`)
5. Fissa la tramoggia sopra l'apertura di carico, lo scivolo sotto quella
   di scarico, il pannello frontale in fondo allo scivolo (adatta
   lunghezza/percorso alla profondità del tuo mobile)
6. Monta pulsante, LED e sensore di caduta secondo `../wiring.md`
7. **Prima di caricare sigarette vere**: fai girare il rullo a vuoto
   qualche ciclo, verifica l'allineamento delle aperture, poi carica
   poche decine di sigarette per un primo collaudo prima di riempire la
   tramoggia del tutto
