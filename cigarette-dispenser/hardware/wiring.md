# Cablaggio

Mappatura GPIO usata da `../software/esphome/cigarette-dispenser.yaml`.
Cambia pure i pin, basta che corrispondano tra questo schema e lo YAML.

## Mappa GPIO (ESP32)

| Segnale | Pin ESP32 | Verso |
|---|---|---|
| Driver stepper — STEP | GPIO25 | uscita |
| Driver stepper — DIR | GPIO26 | uscita |
| Driver stepper — ENABLE | GPIO27 | uscita (attivo basso) |
| Servo paletta scivolo | GPIO13 (PWM) | uscita |
| Fotointerruttore conferma caduta | GPIO34 (input only) | ingresso |
| Sensore homing (hall/microswitch) | GPIO35 (input only) | ingresso |
| PN532 NFC — SDA | GPIO21 | I2C |
| PN532 NFC — SCL | GPIO22 | I2C |
| Buzzer | GPIO32 | uscita |
| LED di stato | GPIO33 | uscita |
| Serratura sportello (via relè/MOSFET) | GPIO14 | uscita |

## Alimentazione — due rail separati

```
                 ┌───────────────┐
  230V AC ──────▶│  PSU 12V 3A   │
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
              │ NEMA17  │  │ ESP32 + servo + sensori +  │
              └─────────┘  │ PN532 + buzzer + LED       │
                            └───────────────────────────┘
```

Tieni il motore passo-passo su un'alimentazione separata (12V direttamente
al driver) dal rail logico a 5V: gli stepper generano rumore elettrico che
può resettare l'ESP32 se condiviso male. Massa comune tra i due rail.

## Driver stepper (A4988/DRV8825)

- VMOT/GND → alimentazione 12V motore (con condensatore elettrolitico
  ≥100µF vicino al driver, come da datasheet)
- 1A/1B/2A/2B → bobine del NEMA17
- STEP/DIR/ENABLE → ESP32 come da tabella
- VDD/GND logica → 3.3V dell'ESP32
- Regola il trimmer di corrente secondo la corrente nominale del tuo
  NEMA17 (di solito 70-100% della corrente nominale bobina)

## Sensori

- Il **fotointerruttore a forcella** va montato sullo scivolo (`base.scad`),
  in modo che il pacchetto lo attraversi cadendo: conferma l'erogazione
  reale, non solo il comando inviato
- Il **sensore di homing** (hall + magnete sul mozzo, o microswitch che
  clicca su una tacca dello spicchio) individua la tasca 0 all'accensione,
  così il software sa sempre quale tasca è allineata allo scivolo anche
  dopo un riavvio

## GPIO input-only

GPIO34 e GPIO35 sono input-only sull'ESP32 (nessun pull-up interno):
aggiungi una resistenza di pull-up esterna da 10kΩ se il tuo sensore è a
collettore aperto (tipico dei fotointerruttori a forcella economici).
