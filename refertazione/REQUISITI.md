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
- **Archivi separati per impostazione predefinita**: ogni medico (o studio) vede solo
  i propri pazienti; la separazione è garantita a livello di database (multi-tenant).
  Più medici possono far parte dello stesso **studio** e condividerne l'archivio.

### Condivisione tra medici (facoltativa)
Nulla è condiviso finché il medico non lo decide esplicitamente.

| Cosa | Come | Note |
|---|---|---|
| Modelli di referto, liste, classificazioni | pubblicazione nella libreria comune | nessun dato di pazienti |
| Singolo paziente / referto | condivisione con un collega specifico | sola lettura, motivo obbligatorio, scadenza opzionale, revocabile |
| Statistiche (es. ADR, numero esami) | dati aggregati e anonimi | confronto tra medici senza dati personali |

- Ogni condivisione e ogni consultazione da parte del collega finisce nel registro accessi.
- Base giuridica: la condivisione di dati clinici è ammessa per finalità di cura quando il
  collega partecipa alla cura del paziente (art. 9.2.h GDPR), con informativa al paziente.
  Per ricerca servono dati anonimi o il consenso del paziente.

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
- Il prodotto sarà offerto anche a medici esterni (eventualmente a pagamento)?
- Quali dati i medici vorranno condividere più spesso?
- Account usato dal medico per l'SSO: Gmail personale, Google Workspace o Microsoft 365?
- Tipi di esame da refertare nella prima versione.
- Funzioni della prima versione.
