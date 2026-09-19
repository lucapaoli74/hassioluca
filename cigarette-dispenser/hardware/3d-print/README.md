# Parti da stampare in 3D

File parametrici OpenSCAD (testati con `openscad 2021.01`, tutte le mesh
esportate sono manifold/chiuse — verificato in questa sessione con
`openscad -o out.stl <file>.scad`).

## File

| File | Cosa stampa | Copie |
|---|---|---|
| `hub.scad` | Mozzo centrale, si accoppia all'albero motore | 1 |
| `wedge.scad` | Tasca/spicchio del carosello | `slots` (6 di default) |
| `base.scad` | Piatto fisso con scivolo e supporto motore | 1 |
| `lid.scad` | Coperchio + sportello scorrevole di carico | 1 + 1 sportello |
| `params.scad` | Parametri condivisi — **modifica solo questo file** | — |
| `helpers.scad` | Funzioni geometriche condivise | — |
| `assembly.scad` | Solo per anteprima in OpenSCAD, non da stampare | — |

Apri `assembly.scad` nella GUI di OpenSCAD per vedere come si incastrano i
pezzi prima di stampare.

## Come funziona

Il carosello (mozzo + spicchi) ruota sopra al piatto fisso (`base.scad`).
Ogni tasca non ha fondo: il fondo è il piatto fisso stesso. Il piatto ha
un solo scivolo (chute) ritagliato in un punto preciso: quando il motore
passo-passo ruota il carosello di `360°/slots`, la tasca allineata con lo
scivolo lascia cadere il pacchetto per gravità. È lo stesso principio dei
dispenser di pillole/monete: nessuna molla, nessuna leva, solo indicizzazione
e gravità — affidabile e semplice da stampare.

Il coperchio copre tutto tranne un'apertura fissa di carico con uno
sportello scorrevole stampato: fai ruotare il carosello (dal pannello
Home Assistant, vedi `../../software`) finché una tasca vuota non è sotto
l'apertura, poi fai scorrere lo sportello e carica un pacchetto.

## IMPORTANTE: misura i tuoi pacchetti prima di stampare

`params.scad` assume un pacchetto rigido standard (55×22×85 mm). Le marche
variano di qualche millimetro — misura i pacchetti che userai davvero e
aggiorna `pack_w`, `pack_d`, `pack_h`, `clear` prima di stampare. Tutte le
altre dimensioni (`r_in`, `r_out`, `hub_od`, il diametro del piatto) sono
derivate automaticamente da questi valori.

Con i valori di default: piatto Ø 198 mm (adatto a letti da 220×220 mm o
più grandi), mozzo Ø 134 mm × 90 mm alto, 6 tasche.

## Impostazioni di stampa consigliate

- Materiale: **PETG** (meglio di PLA per attrito/usura sul foro albero e
  sulle guide dello sportello; l'ABS va bene ma deforma di più)
- Altezza layer: 0.2 mm
- Perimetri: 4-5 (`wall = 2.4mm` in params.scad assume questo)
- Infill: 15-20% è sufficiente (i pezzi non sono strutturalmente caricati,
  è solo la parete a contare)
- Supporti: nessuno per `base.scad`/`wedge.scad`/`hub.scad` se orientati
  come esportati (piatti sulla faccia larga); `lid.scad` potrebbe
  richiedere supporti minimi sotto le guide dello sportello
- Foro albero (`hub_id`, 8 mm di default): stampalo a misura nominale e
  rifinisci con una punta da trapano se necessario — la stampa 3D
  restringe leggermente i fori

## Assemblaggio

1. Stampa 1× `base.scad`, 1× `hub.scad`, `slots`× `wedge.scad`, 1× `lid.scad`
   (include lo sportello)
2. Premi un cuscinetto a sfere skate 608 (o una boccola) nella sede sotto
   `base.scad`
3. Fissa il mozzo all'albero motore con un giunto flessibile (vedi
   `../bom.md`), passando per il cuscinetto della base
4. Avvita ogni spicchio al relativo boss del mozzo (viti M3×10, vedi BOM)
5. Monta il motore sotto la base con le 4 viti M3
6. Posiziona il coperchio sopra e fissalo alla base con distanziali/viti
7. Prima di caricare i pacchetti, esegui una calibrazione: fai girare il
   carosello a vuoto e verifica che ogni tasca si allinei correttamente
   con lo scivolo (vedi `../../software/esphome` per l'homing tramite
   sensore di prossimità)
