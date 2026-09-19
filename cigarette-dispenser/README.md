# Distributore automatico di sigarette — carosello + Home Assistant

Progetto completo per un piccolo distributore a carosello (6 tasche di
default, una confezione a tasca), pensato per **uso personale/domestico**:
gestione remota tramite Home Assistant, con quota giornaliera
configurabile — utile ad esempio come dispenser auto-limitante per
contingentare il proprio consumo, invece che tenere il pacchetto sempre a
portata di mano.

⚠️ **Prima di costruirlo**: se prevedi di usarlo in un negozio o in un
luogo accessibile a terzi (non solo a te), la vendita di tabacco tramite
distributore automatico è regolata per legge in quasi tutti i paesi
(in Italia serve tipicamente un sistema di verifica dell'età per i
distributori esterni non presidiati). Vedi la nota in `hardware/bom.md`.
Questo progetto, così com'è, **non** è un sistema di verifica dell'età
certificato: il tag NFC è un controllo di comodità per uso privato, non
una misura di compliance legale.

## Struttura del repository

```
cigarette-dispenser/
├── hardware/
│   ├── 3d-print/        ← file OpenSCAD parametrici (testati, mesh manifold)
│   │   ├── params.scad     parametri condivisi — modifica solo questo
│   │   ├── helpers.scad    funzioni geometriche condivise
│   │   ├── hub.scad        mozzo centrale (1×)
│   │   ├── wedge.scad      tasca del carosello (× slots)
│   │   ├── base.scad       piatto fisso + scivolo + supporto motore
│   │   ├── lid.scad        coperchio + sportello di carico scorrevole
│   │   ├── assembly.scad   anteprima assemblaggio (non da stampare)
│   │   └── README.md       impostazioni di stampa, assemblaggio
│   ├── bom.md            ← parti standard da comprare
│   └── wiring.md         ← schema di cablaggio, mappa GPIO
└── software/
    ├── esphome/
    │   ├── cigarette-dispenser.yaml   firmware ESP32 (validato con `esphome config`)
    │   └── secrets.yaml.example
    └── home-assistant/
        ├── packages/cigarette_dispenser.yaml   logica: quota, scorte, NFC
        └── dashboard/cigarette-dispenser-view.yaml   pannello di controllo
```

## Come funziona il meccanismo

Un motore passo-passo ruota un carosello a spicchi sopra un piatto fisso.
Ogni tasca non ha fondo: il fondo è il piatto stesso, tranne in un punto
(lo scivolo) dove una paletta servo-comandata lascia cadere il pacchetto
per gravità quando quella tasca viene indicizzata lì. Stesso principio dei
dispenser di pillole: nessuna molla o leva, solo indicizzazione + gravità.
Il caricamento avviene dall'alto, nello stesso punto fisso, attraverso lo
sportello scorrevole del coperchio. Dettagli in `hardware/3d-print/README.md`.

## Come funziona il software

- **ESPHome** (`software/esphome/`) gira sull'ESP32 e sa solo *come*
  muovere il carosello, aprire la paletta e leggere i sensori. Espone tre
  servizi a Home Assistant: `dispense_slot`, `goto_slot_for_loading`,
  `lock_hatch`/`unlock_hatch`.
- **Home Assistant** (`software/home-assistant/`) decide *se* e *quando*:
  tiene il conteggio di quante tasche sono cariche, applica la quota
  giornaliera, gestisce il controllo accessi NFC opzionale, invia
  notifiche di scorte in esaurimento, e mette tutto in una dashboard.

## Quick start

1. Stampa e assembla l'hardware — vedi `hardware/3d-print/README.md`
2. Compra i componenti — vedi `hardware/bom.md`
3. Cablа tutto — vedi `hardware/wiring.md`
4. Copia `software/esphome/secrets.yaml.example` in `secrets.yaml`,
   compilalo, poi `esphome run cigarette-dispenser.yaml`
5. In Home Assistant: aggiungi l'integrazione ESPHome per il dispositivo,
   copia `software/home-assistant/packages/cigarette_dispenser.yaml` nella
   tua cartella `packages/` (richiede `packages: !include_dir_named packages`
   in `configuration.yaml`)
6. Importa `software/home-assistant/dashboard/cigarette-dispenser-view.yaml`
   come vista Lovelace, verifica gli entity_id generati dall'integrazione
   ESPHome (vedi nota nel file) e correggili se serve
7. Prima di caricare pacchetti veri, esegui un ciclo di homing/test a vuoto
   e verifica l'allineamento carosello/scivolo

## Cosa è stato verificato in questa sessione

- Tutti i file `.scad` esportano mesh 3D manifold/chiuse (`openscad -o
  out.stl`), incluso l'assemblaggio completo — nessun errore di geometria
- Le dimensioni derivate garantiscono che ogni tasca sia effettivamente più
  larga del pacchetto configurato (non era così in una prima revisione: il
  mozzo aveva un diametro fisso che non lasciava spazio a sufficienza —
  ora `hub_od`/`r_in` sono calcolati da `pack_w`/`clear`/`slots`)
- Il file ESPHome passa `esphome config` (validazione schema completa,
  YAML + lambda) con `esphome 2026.6.5`
- I file YAML di Home Assistant sono sintatticamente validi (parsing YAML);
  **non** è stata testata su un'istanza Home Assistant reale — verifica gli
  entity_id esatti generati dalla tua integrazione prima di affidarti alla
  dashboard/automazioni
