# Refertazione – requisiti e decisioni

Rifacimento del gestionale di refertazione (oggi su Ninox) di un medico chirurgo
gastroenterologo, ospitato su un server proprio. Il prodotto deve poter essere usato
da **più medici**.

## Ruoli
| Ruolo | Chi | Accesso |
|---|---|---|
| Admin tecnico | sviluppatore/gestore | utenti, configurazione, modelli, backup; dati clinici solo se necessario e sempre tracciati |
| Medico | uno o più utenti | crea, firma e invia i propri referti |

### Più medici
- Ogni medico ha il proprio profilo: intestazione, logo, firma, modelli di referto.
- Ogni referto è firmato dal medico che lo ha redatto e resta a lui attribuito.
- Il registro degli accessi indica sempre quale medico ha fatto cosa.
- La separazione dei pazienti dipende dal modello scelto (vedi "Da decidere"):
  - **stesso studio**: archivio pazienti condiviso tra i medici dello studio;
  - **medici indipendenti**: ogni medico (o studio) vede solo i propri pazienti,
    con separazione garantita a livello di database (multi-tenant).

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
- Più medici: stesso studio con pazienti condivisi, oppure medici/studi indipendenti
  (ognuno titolare dei propri dati)? Il prodotto sarà offerto anche a medici esterni?
- Account usato dal medico per l'SSO: Gmail personale, Google Workspace o Microsoft 365?
- Tipi di esame da refertare nella prima versione.
- Funzioni della prima versione.
