# Refertazione – requisiti e decisioni

Rifacimento del gestionale di refertazione (oggi su Ninox) di un medico chirurgo
gastroenterologo, ospitato su un server proprio, ed esteso a **qualsiasi specializzazione**:
ogni medico referta le prestazioni della propria disciplina. Il prodotto deve poter essere usato
da **più medici**. Per ora è **gratuito**; in futuro potrebbe diventare un servizio a
pagamento. La struttura deve essere **molto robusta** fin dall'inizio.

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

## Modelli di referto configurabili (multi-specializzazione)
I tipi di esame **non sono scritti nel codice**: sono configurazione.

- **Specializzazione** → **Tipo di prestazione** (es. Gastroenterologia → Colonscopia)
  → **Modello di referto** con i suoi campi.
- Tipi di campo: testo libero, testo con frasi predefinite, numero con unità e intervalli,
  data, scelta singola/multipla, classificazione/score, misure, immagini, tabelle ripetibili
  (es. un polipo per riga), campi calcolati (solo score standard, non suggerimenti diagnostici).
- **Classificazioni riutilizzabili** come cataloghi (es. Boston, Paris, Los Angeles, Forrest,
  Mayo) condivise tra modelli.
- **Modelli versionati**: un referto firmato resta legato alla versione del modello con cui
  è stato scritto; modificare un modello non altera i referti già firmati.
- Ogni referto salva sia i **dati strutturati** (validati sul modello) sia il **testo
  finale**, così statistiche e ricerche funzionano su qualsiasi specializzazione.
- Editor dei modelli per admin e medici; libreria comune di modelli condivisibili.
- Primo set di modelli: gastroenterologia/chirurgia, ricavati dal database Ninox attuale.
- Predisposizione a codifiche standard (ICD-9-CM, nomenclatore prestazioni, LOINC/FHIR).
- Attenzione MDR: niente diagnosi o suggerimenti clinici automatici, per non rientrare
  nei dispositivi medici software.

## Accesso (login)
- Il medico accede con **SSO Google o Microsoft** (OAuth2/OpenID Connect, via django-allauth).
- **Nessuna registrazione libera**: entra solo un indirizzo email già creato dall'admin
  (lista consentita). Un account Google/Microsoft qualsiasi viene rifiutato.
- Account usati dai medici: **Gmail personale** e **Microsoft 365** (account di lavoro).
- **Gmail personale**: l'app non può verificare né imporre la verifica in due passaggi
  di Google. Per questo, dopo il login Google, l'app chiede un **secondo fattore via email**:
  codice di 6 cifre, valido 10 minuti, monouso, massimo 5 tentativi, invio limitato nel tempo.
  - Il codice va a un **indirizzo diverso** da quello usato per il login (es. email dello
    studio o PEC): inviarlo alla stessa Gmail non proteggerebbe da un furto dell'account Gmail.
  - Richiesto a ogni nuovo dispositivo; dispositivo fidato per 30 giorni (revocabile).
  - L'email contiene solo il codice, nessun dato clinico. Invio tramite provider SMTP con
    server nell'UE.
- **Microsoft 365**: app Entra ID multi-tenant limitata agli account di lavoro
  (endpoint `organizations`, niente account Microsoft personali). La verifica in due
  passaggi è imposta dal tenant del medico (Security defaults / accesso condizionale);
  l'amministratore del tenant potrebbe dover approvare l'app una volta.
- Il collegamento account ↔ medico usa l'identificativo stabile del provider
  (Google `sub`, Microsoft `tid`+`oid`), non solo l'email, che può essere riassegnata.
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

## Robustezza (requisiti non negoziabili)
**Isolamento tra medici**
- Doppio controllo: filtro nell'applicazione + Row-Level Security di PostgreSQL, così un
  errore nel codice non può mostrare i pazienti di un altro studio.
- Test automatici dedicati che verificano l'isolamento a ogni modifica.

**Integrità dei dati**
- Referti firmati immutabili, con versioni e impronta (hash) del contenuto; PDF/A,
  predisposto per la firma digitale qualificata (PAdES).
- Registro accessi in sola aggiunta, con catena di hash per rilevare manomissioni.
- Migrazioni del database versionate, mai modifiche manuali in produzione.

**Backup e ripristino**
- Regola 3-2-1: database con ripristino a un punto nel tempo (archiviazione WAL, perdita
  massima di pochi minuti) + copie notturne cifrate in un altro data center.
- Prova di ripristino automatica periodica: un backup mai ripristinato non conta.

**Sicurezza**
- Disco e backup cifrati; immagini endoscopiche in object storage UE cifrato.
- Server: solo chiavi SSH, firewall, aggiornamenti di sicurezza automatici, fail2ban.
- Applicazione: limiti ai tentativi, intestazioni di sicurezza (CSP, HSTS), controllo
  automatico delle dipendenze vulnerabili.
- Nessun dato clinico verso servizi terzi (log ed errori ripuliti dai dati personali).

**Qualità e rilascio**
- Test automatici e controlli a ogni modifica (GitHub Actions).
- Ambiente di prova (staging) separato dalla produzione, con soli dati fittizi.
- Monitoraggio di disponibilità ed errori, con avvisi all'admin.

**Pronto a crescere**
- Applicazione senza stato sul server: si può passare a più server o a un database
  gestito senza riscriverla.
- Ogni studio ha già un "piano" associato, per introdurre in futuro abbonamenti.
- Esportazione dei dati di un medico in formato aperto (portabilità GDPR), predisposta
  per standard sanitari (HL7 FHIR) in vista di integrazioni future.

**Adempimenti**
- Valutazione d'impatto (DPIA), registro dei trattamenti, accordi art. 28 con ogni
  medico/studio e con i fornitori, informativa ai pazienti, procedura data breach (72 ore).

## Infrastruttura proposta
- VPS Hetzner Cloud (Germania/Finlandia), in alternativa Aruba Cloud (Italia).
- Backup su Storage Box in un altro data center; object storage UE per le immagini.
- Due ambienti: produzione e staging.
- Django + PostgreSQL, PDF dei referti, Docker, Caddy (HTTPS automatico).
- Il server si acquista al momento del collaudo.

## Origine dati: Ninox
- Si estrae via API solo la struttura (`ninox/esporta_schema.py`), nessun record.
- Serve `NINOX_API_KEY` nelle impostazioni dell'ambiente e `api.ninox.com` tra i domini consentiti.

## Da decidere
- Conferma: codice del secondo fattore a un indirizzo email diverso da quello di login.
- Repository dedicato e privato per il progetto.
- Quali dati i medici vorranno condividere più spesso?
- Tipi di esame da refertare nella prima versione.
- Funzioni della prima versione.
