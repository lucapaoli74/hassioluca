# App Android

Non ho scritto un'app Android nativa da zero: per controllare un dispositivo
Home Assistant esiste già un'app ufficiale, gratuita e open source — la
**Home Assistant Companion App per Android** — che è quasi sempre la scelta
giusta invece di reinventarla. Ecco cosa ti dà, gratis, per il lavoro già
fatto in questo repository:

- accesso alla dashboard (`software/home-assistant/dashboard/`) e quindi al
  pulsante "Eroga una sigaretta", alla quota giornaliera, alla scorta
  stimata, allo storico erogazioni
- **notifiche push** sul telefono per gli eventi che il pacchetto già genera
  (`esphome.dispense_confirmed`, `esphome.dispense_failed`,
  `esphome.quota_reached`) — basta creare un'azione `notify.mobile_app_<tuo_telefono>`
  nelle automazioni di `packages/cigarette_dispenser.yaml` al posto (o in
  aggiunta) di `persistent_notification.create`
- widget per la schermata home (es. un pulsante rapido "Eroga una sigaretta")
- accesso sia da rete locale che da remoto (con Nabu Casa o il tuo
  reverse proxy)

## Attivazione

1. Installa "Home Assistant" dal Play Store sul telefono
2. Accedi con l'account della tua istanza Home Assistant (locale o remota)
3. Apri la dashboard "Distributore Sigarette" (quella importata da
   `software/home-assistant/dashboard/cigarette-dispenser-view.yaml`)
4. Per le notifiche push: Impostazioni → Compagno → Notifiche, poi nel
   pacchetto YAML sostituisci `persistent_notification.create` con
   `notify.mobile_app_<nome_dispositivo>` (il nome esatto lo trovi in
   Impostazioni → Dispositivi e servizi → Mobile App)
5. Per un widget: tieni premuto sulla home del telefono → Widget → Home
   Assistant → scegli l'entità `esphome.cigarette_dispenser_dispense_one`
   o un pulsante della dashboard

## Se invece vuoi un'app dedicata (brandizzata, standalone)

Questa è una scelta di scope diversa e più grande: significa sviluppare e
mantenere un'app separata (nativa Kotlin, o una PWA installabile, o
React Native) invece di riusare quella esistente. Ha senso solo se ti serve
un'esperienza completamente diversa dalla dashboard di Home Assistant (per
esempio: un'interfaccia super-semplificata per un altro utente che non deve
vedere il resto della tua casa). Se è quello che vuoi, fammelo sapere e la
impostiamo come progetto a parte — non l'ho inclusa qui per non consegnarti
un'app abbozzata e poco affidabile al posto di una già pronta e testata.
