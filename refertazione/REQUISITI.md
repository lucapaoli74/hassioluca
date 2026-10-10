# Refertazione – requisiti e decisioni

Rifacimento del gestionale di refertazione (oggi su Ninox) di un medico chirurgo
gastroenterologo, ospitato su un server proprio.

## Ruoli
| Ruolo | Chi | Accesso |
|---|---|---|
| Admin tecnico | sviluppatore/gestore | utenti, configurazione, modelli, backup; dati clinici solo se necessario e sempre tracciati |
| Medico | utente | crea, firma e invia i referti |

Sul piano GDPR il medico è titolare del trattamento; l'admin è responsabile del
trattamento (art. 28): serve un accordo scritto. Lo stesso vale per il fornitore del server.

## Accesso (login)
- Il medico accede con **SSO Google o Microsoft** (OAuth2/OpenID Connect, via django-allauth).
- **Nessuna registrazione libera**: entra solo un indirizzo email già creato dall'admin
  (lista consentita). Un account Google/Microsoft qualsiasi viene rifiutato.
- La verifica in due passaggi va attivata sull'account Google/Microsoft del medico
  (obbligatoria se si usa Google Workspace o Microsoft 365, impostabile dal tenant).
- L'admin mantiene un **accesso locale di emergenza** (password + codice TOTP), per quando
  l'SSO non è disponibile o l'account del medico è bloccato.
- Le sessioni scadono dopo un periodo di inattività. Ogni login viene registrato.
- Google e Microsoft ricevono solo l'autenticazione, non i dati clinici.

## Dati e sicurezza
- Dati sanitari (art. 9 GDPR): server nell'UE, HTTPS, cifratura, backup notturni cifrati
  in un data center diverso, registro degli accessi.
- Referto non modificabile dopo la firma: le correzioni creano una nuova versione.
- Sviluppo e collaudo solo con dati fittizi; i dati reali migrano direttamente da Ninox
  al server di produzione.

## Infrastruttura proposta
- VPS Hetzner Cloud (Germania/Finlandia), in alternativa Aruba Cloud (Italia).
- Backup su Storage Box in un altro data center.
- Django + PostgreSQL, PDF dei referti, Docker, Caddy (HTTPS automatico).
- Il server si acquista al momento del collaudo.

## Origine dati: Ninox
- Si estrae via API solo la struttura (`ninox/esporta_schema.py`), nessun record.
- Serve `NINOX_API_KEY` nelle impostazioni dell'ambiente e `api.ninox.com` tra i domini consentiti.

## Da decidere
- Account usato dal medico per l'SSO: Gmail personale, Google Workspace o Microsoft 365?
- Tipi di esame da refertare nella prima versione.
- Funzioni della prima versione.
