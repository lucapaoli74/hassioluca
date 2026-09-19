# Cablaggio

Mappatura GPIO usata da `../software/esphome/cigarette-dispenser.yaml`.
Cambia pure i pin, basta che corrispondano tra questo schema e lo YAML.

## Mappa GPIO (ESP32)

| Segnale | Pin ESP32 | Verso |
|---|---|---|
| Driver stepper — STEP | GPIO25 | uscita |
| Driver stepper — DIR | GPIO26 | uscita |
| Driver stepper — SLEEP | GPIO27 | uscita (attivo basso) |
| Pulsante frontale erogazione | GPIO4 | ingresso |
| Fotointerruttore conferma caduta | GPIO34 (input only) | ingresso |
| Buzzer | GPIO32 | uscita |
| LED di stato | GPIO33 | uscita |
| Serratura tramoggia (via relè/MOSFET) | GPIO14 | uscita |

## Alimentazione — due rail separati

```
                 ┌───────────────┐
  230V AC ──────▶│  PSU 12V 2A   │
                 └───┬───────┬───┘
                     │12V    │12V
                     ▼       ▼
              ┌─────────┐ ┌──────────────┐
              │ Driver  │ │ Buck 12V→5V  │
              │ stepper │ │   3A         │
              └────┬────┘ └──────┬───────┘
                   │ motore      │5V
                   ▼             ▼
              ┌─────────┐  ┌───────────────────────────┐
              │ NEMA17  │  │ ESP32 + pulsante + sensore│
              └─────────┘  │ + buzzer + LED + serratura│
                            └───────────────────────────┘
```

Tieni il motore passo-passo su un'alimentazione separata (12V direttamente
al driver) dal rail logico a 5V: gli stepper generano rumore elettrico che
può resettare l'ESP32 se condiviso male. Massa comune tra i due rail.

## Driver stepper (A4988/DRV8825)

- VMOT/GND → alimentazione 12V motore (con condensatore elettrolitico
  ≥100µF vicino al driver, come da datasheet)
- 1A/1B/2A/2B → bobine del NEMA17
- STEP/DIR/SLEEP → ESP32 come da tabella
- VDD/GND logica → 3.3V dell'ESP32
- Regola il trimmer di corrente secondo la corrente nominale del tuo
  NEMA17 (di solito 70-100% della corrente nominale bobina) — il rullo ha
  carico leggero, non serve spingere la corrente al massimo

## Pulsante frontale

Pulsante momentaneo normalmente aperto tra GPIO4 e GND. Il firmware usa
`INPUT_PULLUP` + `inverted: true`, quindi non serve resistenza esterna: a
riposo il pin legge alto (interruttore aperto → non premuto), a massa
quando premuto.

## Sensore di caduta

Il **fotointerruttore a forcella** va montato sullo scivolo (`chute.scad`),
in modo che la sigaretta lo attraversi cadendo: conferma l'erogazione
reale, non solo il comando inviato. È la rete di sicurezza software che
distingue un'erogazione riuscita da un inceppamento o da una tramoggia
vuota (eventi `esphome.dispense_confirmed` / `esphome.dispense_failed`,
vedi il pacchetto Home Assistant).

GPIO34 è input-only sull'ESP32 (nessun pull-up interno): aggiungi una
resistenza di pull-up esterna da 10kΩ se il tuo sensore è a collettore
aperto (tipico dei fotointerruttori a forcella economici).

## Nota su homing/allineamento

A differenza di una prima versione basata su tasche numerate, il rullo
scanalato non ha bisogno di sapere "quale" scanalatura è in posizione: sono
tutte identiche, quindi ogni pressione del pulsante fa semplicemente
avanzare il rullo di `360°/flutes`. Non è cablato un sensore di home in
questa versione. Se con l'uso noti una deriva (passi persi dal motore),
puoi aggiungere un sensore hall + magnete o un microswitch come riferimento
periodico — non incluso di default per tenere il cablaggio semplice.
