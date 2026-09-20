# Parti da stampare in 3D — meccanismo per sigarette sfuse

File parametrici OpenSCAD (testati con `openscad 2021.01`, ogni pezzo
esporta una mesh manifold/chiusa — verificato in sessione con
`openscad -o out.stl <file>.scad`; l'assemblaggio completo è stato
renderizzato senza errori).

**Tutti i pezzi, mobile compreso, entrano nel piano di stampa di una Bambu
X1C** (256×256×256mm — verificato leggendo il bounding box reale di ogni
STL/DXF esportato, margine di sicurezza a 250mm in `x1c_max`,
`params.scad`). I pannelli del mobile più alti di 250mm (`cabinet.scad`,
lati e retro) sono spezzati in due segmenti che si avvitano insieme lungo
la giunzione — vedi la sezione Assemblaggio.

**Il mobile non è più un guscio a misura indovinata**: le sue quote
(`cab_w`/`cab_d`/`cab_h` in `params.scad`) sono calcolate dalla geometria
reale del meccanismo (rullo, housing, tramoggia, scivolo), e i pannelli
hanno i fori giusti per avvitarci sopra housing e tramoggia — non solo
per contenerli a vista. Con i valori di default il mobile è
**132×128×316mm** (storia delle riduzioni: 180×240×420mm con margini a
occhio → 154×130×363mm col primo calcolo puntuale → questi 132×128×316mm
dopo aver ridotto la tramoggia stessa e i margini al minimo che rispetta
ancora sia la capienza richiesta sia i vincoli geometrici — vedi sotto).
Vedi `assembly.scad` per la vista d'insieme quotata.

### Fin dove si può ridurre: i tre vincoli che contano

Ridurre "a piacere" `hopper_top_w/d/h` e i margini del mobile in
`params.scad` produce in fretta geometrie che sembrano più piccole ma non
sono valide. Le quote di default sono il risultato di una ricerca numerica
(non a tentativi) che rispetta questi vincoli, tutti verificati in sessione
sui file `.scad` risultanti, non solo calcolati a mano:

1. **Capienza**: il volume utile della tramoggia (integrale del tronco di
   piramide tra `hopper_bot_*` e `hopper_top_*`) per il fattore di
   impaccamento pessimistico (35%) deve restare >=~100 sigarette — con i
   valori di default sono **~102** (fino a ~146 al 50%, scenario più
   realistico).
2. **Fori d'angolo della tramoggia**: il pannello superiore/i pannelli
   laterali hanno fori Ø4mm nei punti in cui il bordo della tramoggia
   (`rim_w`) li combacia — se `side_wall_margin`/`depth_margin` sono troppo
   stretti rispetto a `hopper_top_w`/`hopper_top_d`, quei fori sfondano il
   bordo del pannello invece di restarci dentro. `cab_w`/`cab_d` in
   `params.scad` includono ora questo vincolo esplicitamente
   (`corner_hole_edge_margin`), non solo un margine forfettario.
3. **Feritoia del pannello frontale**: `slot_w = drop_w + 8` più i fori
   d'angolo di fissaggio del pannello non possono essere più larghi del
   pannello stesso — anche questo è nella formula di `cab_w`, non lasciato
   a un margine indovinato.

Se cambi `cig_l`/`cig_d` (sigarette diverse) o `flutes`/`wall` (rullo
diverso), tutte queste quote si ricalcolano da sole — ma rifai comunque la
verifica di questi tre vincoli (bastano gli stessi comandi usati in
sessione: `openscad -o out.stl <file>.scad` per la mesh, e un rendering di
`assembly.scad` per vedere a occhio se qualcosa si sovrappone).

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
| `chute.scad` | Scivolo a labirinto, obliquo (collega housing e pannello frontale) | 1 | stampa 3D |
| `front_panel.scad` | Pannello con pulsante, LED, feritoia — copre solo la parte bassa del mobile | 1 | stampa 3D o dima per taglio |
| `cabinet.scad` | Pannelli del mobile, con fori di fissaggio per housing e tramoggia (sopra/sotto + fianchi/retro spezzati in 2) | 8 | stampa 3D, o taglio laser/CNC/sega (esporta `.dxf`) |
| `params.scad` | Parametri condivisi — **modifica solo questo file** | — | — |
| `helpers.scad` | Funzioni geometriche condivise (incluse le lettere incise, vedi sotto) | — | — |
| `assembly.scad` | Solo anteprima in OpenSCAD, non da stampare | — | — |

## Legenda pezzi (A-O): una lettera incisa su ogni pezzo

I 15 pezzi stampabili hanno tutti una lettera incisa (o su una piccola
targhetta sporgente, dove il pezzo è troppo sottile per un'incisione) così
si riconoscono senza doverli confrontare a occhio col disegno mentre li
stacchi dal piatto di stampa — modulo `label_cut`/`label_tag` in
`helpers.scad`, usato da ogni file di questa cartella.

| Lettera | Pezzo | File | Dove è incisa |
|---|---|---|---|
| **A** | Rullo scanalato | `roller.scad` | faccia piatta di un'estremità, tra il foro albero e il fondo delle scanalature |
| **B** | Alloggiamento a "C" | `housing.scad` | faccia esterna del piatto folle (lato opposto al motore) |
| **C** | Griglia di sicurezza | `grille.scad` | targhetta sporgente sul bordo posteriore del telaio |
| **D** | Tramoggia | `hopper.scad` | angolo posteriore destro del bordo superiore |
| **E** | Coperchio tramoggia | `hopper_lid.scad` | angolo posteriore destro della faccia superiore |
| **F** | Scivolo | `chute.scad` | targhetta sporgente sulla parete esterna, vicino alla cima |
| **G** | Pannello frontale | `front_panel.scad` | faccia interna, tra la feritoia e il pulsante |
| **H** | Pannello superiore | `cabinet.scad` → `top_panel_3d()` | accanto al ritaglio della tramoggia |
| **I** | Pannello inferiore | `cabinet.scad` → `bottom_panel_3d()` | centro pannello |
| **J** | Fianco motore, segmento inferiore | `cabinet.scad` → `side_panel_seg_3d(0,true,...)` | vicino al cerchio di fori housing/NEMA17 |
| **K** | Fianco motore, segmento superiore | `cabinet.scad` → `side_panel_seg_3d(1,true,...)` | centro pannello |
| **L** | Fianco folle, segmento inferiore | `cabinet.scad` → `side_panel_seg_3d(0,false,...)` | vicino al cerchio di fori housing |
| **M** | Fianco folle, segmento superiore | `cabinet.scad` → `side_panel_seg_3d(1,false,...)` | centro pannello |
| **N** | Retro, segmento inferiore | `cabinet.scad` → `back_panel_seg_3d(0,...)` | sopra il foro passacavi |
| **O** | Retro, segmento superiore | `cabinet.scad` → `back_panel_seg_3d(1,...)` | centro pannello |

**Nota per `cabinet.scad`**: i moduli 2D originali (`top_panel()`,
`side_panel_seg(i, is_drive)`, ecc.) restano invariati e senza lettera —
servono anche per l'export `.dxf` da taglio, dove un'incisione non ha
senso. Le versioni `_3d()` aggiunte in fondo al file estrudono a
`panel_mat_t` e incidono la lettera: usa quelle per l'export STL/3MF da
stampa 3D.

Schema di montaggio con questa stessa legenda (dove va ogni lettera,
passo-passo): vedi l'artifact "Guida al montaggio" condiviso in
conversazione, o riproducilo dalla sezione Assemblaggio più sotto.

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

Con i valori di default: rullo Ø 36.7mm × 88mm, alloggiamento Ø ~50.7mm,
tramoggia con capacità stimata 102-146 sigarette a seconda di quanto si
impaccano (vedi il calcolo commentato in `params.scad`) — sopra le 100
richieste anche nello scenario pessimistico (35%), ma con meno margine di
prima: la tramoggia è stata ridotta al minimo che rispetta ancora quella
soglia, non più sovradimensionata "per sicurezza". Se preferisci più
scorta a scapito dell'ingombro, aumenta `hopper_h` in `params.scad` (è la
dimensione più economica da crescere: aggiunge capacità senza spostare i
tre vincoli geometrici della sezione precedente).

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

## Come si ancora il meccanismo al mobile

Non è un guscio generico: i pannelli hanno i fori esatti per i punti di
fissaggio che housing e tramoggia già hanno.

- **Housing → pannelli laterali**: l'housing ha 4 orecchiette con foro su
  ciascuno dei due piatti terminali (`mount_ear_r`/`mount_hole_r` in
  `params.scad`). I pannelli laterali del mobile hanno lo **stesso
  identico pattern di 4 fori**, più un foro centrale per l'albero — quindi
  l'housing si avvita direttamente ai due fianchi, non "da qualche parte
  dentro alla scatola". Il fianco lato motore ha in più il pattern NEMA17
  (passo 31mm) e un foro Ø24mm per l'albero: **il motore resta fuori dal
  mobile**, avvitato da fuori, solo l'albero entra — niente bisogno di
  fargli spazio dentro.
- **Tramoggia → pannello superiore**: il pannello superiore ha 4 fori
  d'angolo allineati esattamente agli angoli del bordo della tramoggia
  (`hopper.scad`), oltre al ritaglio che la fa sporgere.
- **Scivolo → housing/pannello frontale**: lo scivolo non è più un
  imbuto dritto "da adattare" — è disegnato obliquo (`chute_dy` in
  params.scad) per collegare esattamente lo scarico dell'housing (al
  centro del mobile) alla feritoia del pannello frontale (vicino al
  fronte). Un paio di mm di gioco alla giunzione vanno comunque rifiniti/
  incollati a mano, non è una tolleranza da lavorazione CNC.
- **Pannello frontale**: copre solo la parte bassa del mobile (fino a
  `front_panel_h`, sotto i 250mm — non spezzato), quindi la tramoggia
  resta "a vista" sopra, chiusa dalle sue stesse pareti piene.

Vedi `assembly.scad` per la vista 3D con tutte queste quote applicate.

## Assemblaggio

Lettere tra parentesi = legenda A-O qui sopra.

1. Stampa 1× ciascuno di **A** `roller.scad`, **B** `housing.scad`,
   **C** `grille.scad`, **D** `hopper.scad`, **E** `hopper_lid.scad`,
   **F** `chute.scad`, **G** `front_panel.scad`; stampa (o taglia) gli 8
   pezzi di `cabinet.scad` (**H** sopra, **I** sotto, **J**+**K** fianco
   motore in 2 segmenti, **L**+**M** fianco folle in 2 segmenti, **N**+**O**
   retro in 2 segmenti). Unisci ogni coppia di segmenti (**J**+**K**,
   **L**+**M**, **N**+**O**) con viti M4 nei fori allineati lungo la
   giunzione, con una fascetta/listello interno a cavallo della cucitura
   per irrigidirla
2. Monta **A** dentro **B**, verifica che giri libero senza attrito
   eccessivo contro le pareti
3. Avvita **B** ai pannelli **J**+**K** e **L**+**M** usando i fori delle
   orecchiette (stesso pattern su housing e pannelli, vedi sopra)
4. Avvita il motore da fuori su **J** (pattern NEMA17), collega l'albero a
   **B** con un giunto flessibile attraverso il foro Ø24mm
5. Il perno "idler" scorre nel foro di passaggio su **L** — se vuoi più
   durata, aggiungi un piccolo cuscinetto lì (vedi `../bom.md`)
6. Completa il guscio: **I** (fondo), **N**+**O** (posteriore), angolari
   agli spigoli (vedi `../bom.md`)
7. Incastra **C** tra la bocca inferiore di **D** e l'apertura di carico
   di **B**, poi avvita **D** ai 4 fori d'angolo di **H**
8. Fissa **F** sotto l'apertura di scarico di **B**; il suo fondo arriva
   vicino alla feritoia di **G** — rifinisci/incolla la giunzione
9. Avvita **G** in fondo al mobile
10. Monta **E** con la cerniera sul lato fronte, la serratura + il sensore
    magnetico sul lato retro (fori già presenti nel bordo stampato)
11. Monta pulsante, LED, sensore di caduta e sensore sportello secondo
    `../wiring.md`
12. **Prima di caricare sigarette vere**: fai girare **A** a vuoto
    qualche ciclo (con lo sportello chiuso, altrimenti l'interblocco lo
    impedisce), verifica l'allineamento delle aperture, poi carica poche
    decine di sigarette per un primo collaudo prima di riempire la
    tramoggia del tutto
