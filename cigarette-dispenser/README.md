# Distributore automatico di sigarette sfuse — rullo + Home Assistant

Distributore per **sigarette sfuse** (non in pacchetto): tramoggia da
almeno 100 pezzi, pulsante frontale che eroga **una sigaretta alla volta**,
politica di quota giornaliera impostabile da pannello web (es. 10/giorno) e
controllo/notifiche anche da app Android — pensato per **uso
personale/domestico**, es. come dispenser auto-limitante per contingentare
il proprio consumo.

⚠️ **Prima di costruirlo**: se prevedi di usarlo in un negozio o in un
luogo accessibile a terzi (non solo a te), la vendita di tabacco tramite
distributore automatico è regolata per legge in quasi tutti i paesi (in
Italia serve tipicamente un sistema di verifica dell'età per i
distributori esterni non presidiati). Vedi la nota in `hardware/bom.md`.
Questo progetto, così com'è, **non** è un sistema di verifica dell'età
certificato.

## Struttura del repository

```
cigarette-dispenser/
├── hardware/
│   ├── 3d-print/            ← file OpenSCAD parametrici (testati, mesh manifold)
│   │   ├── params.scad         parametri condivisi — modifica solo questo
│   │   ├── helpers.scad        funzioni geometriche condivise
│   │   ├── roller.scad         rullo scanalato (singolarizzatore, 1×)
│   │   ├── housing.scad        alloggiamento a "C" + piatti terminali
│   │   ├── grille.scad         griglia di sicurezza tramoggia→rullo
│   │   ├── hopper.scad         tramoggia (>=100 sigarette sfuse)
│   │   ├── hopper_lid.scad     coperchio tramoggia (cerniera + serratura)
│   │   ├── chute.scad          scivolo a labirinto anti-intrusione
│   │   ├── front_panel.scad    pannello con pulsante, LED, feritoia
│   │   ├── cabinet.scad        pannelli del mobile (taglio laser/CNC)
│   │   ├── assembly.scad       anteprima assemblaggio (non da stampare)
│   │   └── README.md           come funziona, sicurezza, impostazioni di stampa
│   ├── bom.md                ← parti standard da comprare
│   └── wiring.md             ← schema di cablaggio, mappa GPIO
└── software/
    ├── esphome/
    │   ├── cigarette-dispenser.yaml   firmware ESP32 (validato con `esphome config`)
    │   └── secrets.yaml.example
    ├── home-assistant/
    │   ├── packages/cigarette_dispenser.yaml   notifiche, scorta in esaurimento
    │   └── dashboard/cigarette-dispenser-view.yaml   pannello di controllo
    └── android-app.md        ← come usare l'app Android (Companion App)
```

## Come funziona il meccanismo

Le sigarette stanno sfuse nella **tramoggia** e un **rullo scanalato**
(stesso principio dei contapillole farmaceutici) ne cattura una alla volta
mentre ruota: un **alloggiamento a "C"** trattiene quella catturata e fa
da pettine contro doppie/incastri, finché la scanalatura non raggiunge
l'apertura di scarico e la sigaretta cade per gravità nello **scivolo**,
fino al **pannello frontale**. Ogni pressione del pulsante fa avanzare il
rullo di esattamente una scanalatura = una sigaretta. Dettagli e limiti
(serve una taratura empirica, come per qualunque dispenser di oggetti alla
rinfusa) in `hardware/3d-print/README.md`.

## Sicurezza

Il meccanismo va sempre chiuso in un **mobile** (pannelli quotati in
`hardware/3d-print/cabinet.scad`), con tre misure aggiuntive contro
l'intrappolamento delle dita: una griglia tra tramoggia e rullo, uno
scivolo a labirinto tra rullo e feritoia, e uno sportello di ricarica con
serratura elettrica **il cui sensore blocca il firmware dal muovere il
motore finché non è chiuso** — vale per il pulsante fisico, la dashboard e
l'app, sempre. Dettagli in `hardware/3d-print/README.md` e
`hardware/bom.md`. Non è una certificazione di sicurezza macchine
(ISO 13857/EN60204): sono accorgimenti ragionevoli per un uso domestico
consapevole, non per un contesto con bambini piccoli senza supervisione.

## Come funziona il software

- **ESPHome** (`software/esphome/`) gira sull'ESP32 ed è l'unica autorità
  che decide se erogare: tiene la **quota giornaliera** e il conteggio di
  oggi *sul dispositivo* (non in Home Assistant), con reset a mezzanotte.
  Pulsante fisico, dashboard e app Android chiamano tutti la stessa
  logica — la politica vale sempre, anche se il WiFi cade per un attimo.
- **Home Assistant** (`software/home-assistant/`) è il pannello web: legge
  scorta/quota/conteggio in tempo reale, ti fa cambiare la quota (es. da 10
  a un altro valore) e la scorta stimata dopo una ricarica, invia notifiche
  (erogazione, quota raggiunta, scorta in esaurimento, inceppamento).
- **Android**: niente app custom — si usa la **Home Assistant Companion
  App** ufficiale, che ti dà dashboard, notifiche push e widget gratis
  sopra a quanto già costruito qui. Vedi `software/android-app.md`.

## Quick start

1. Stampa e assembla l'hardware — vedi `hardware/3d-print/README.md`
2. Compra i componenti — vedi `hardware/bom.md`
3. Cabla tutto — vedi `hardware/wiring.md`
4. Copia `software/esphome/secrets.yaml.example` in `secrets.yaml`,
   compilalo, poi `esphome run cigarette-dispenser.yaml`
5. In Home Assistant: aggiungi l'integrazione ESPHome per il dispositivo,
   copia `software/home-assistant/packages/cigarette_dispenser.yaml` nella
   tua cartella `packages/` (richiede `packages: !include_dir_named packages`
   in `configuration.yaml`)
6. Importa `software/home-assistant/dashboard/cigarette-dispenser-view.yaml`
   come vista Lovelace, verifica gli entity_id generati dall'integrazione
   ESPHome (vedi nota nel file) e correggili se serve
7. Imposta la quota giornaliera dal pannello (`number.quota_giornaliera`,
   default 10) e la scorta dopo il primo caricamento
   (`number.scorta_stimata`)
8. Installa la Home Assistant Companion App su Android — vedi
   `software/android-app.md`
9. **Prima di caricare sigarette vere**: fai girare il rullo a vuoto (con
   lo sportello tramoggia chiuso — il firmware si rifiuta di muovere il
   motore altrimenti, vedi la sezione Sicurezza sotto), verifica
   l'allineamento, poi collauda con poche decine di sigarette prima di
   riempire la tramoggia del tutto

## Cosa è stato verificato in questa sessione

- Tutti i file `.scad` esportano mesh 3D manifold/chiuse (`openscad -o
  out.stl`), incluso l'assemblaggio completo — nessun errore di geometria
  (in una prima versione un rullo scanalato era tangente alla superficie
  in un solo punto invece di formare un canale aperto: corretto)
- La capacità della tramoggia è stata calcolata numericamente (integrale
  del volume utile × fattore di impaccamento realistico per cilindretti
  alla rinfusa) per garantire >=100 sigarette anche nello scenario
  pessimistico (35% di impaccamento) — vedi i commenti in `params.scad`
- Il file ESPHome passa `esphome config` (validazione schema completa,
  YAML + lambda) con `esphome 2026.6.5`. Una compilazione binaria completa
  non è stata possibile in questa sessione per un problema di rete del
  sandbox (verifica certificato TLS nel download del toolchain
  Espressif) non legato alla correttezza della configurazione — verificala
  con `esphome compile` nel tuo ambiente prima del primo flash
- I file YAML di Home Assistant sono sintatticamente validi (parsing YAML);
  **non** è stata testata su un'istanza Home Assistant reale — verifica gli
  entity_id esatti generati dalla tua integrazione prima di affidarti alla
  dashboard/automazioni
- Aggiunta la chiusura di sicurezza: griglia (`grille.scad`), scivolo a
  labirinto (`chute.scad`, alette verificate con un rendering in sezione),
  sportello con serratura e interblocco firmware (sensore letto da
  `try_dispense` prima di ogni movimento), mobile a pannelli piatti
  (`cabinet.scad`, con export `.dxf` verificato leggendo le coordinate
  reali dei contorni). Tutti i nuovi/modificati file `.scad` restano mesh
  manifold; il firmware aggiornato passa di nuovo `esphome config`
