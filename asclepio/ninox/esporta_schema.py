#!/usr/bin/env python3
"""Esporta la STRUTTURA di un database Ninox (tabelle, campi, tipi, relazioni).

Non scarica nessun record: i dati dei pazienti non vengono mai letti.

Uso:
    export NINOX_API_KEY=...            # chiave API Ninox (mai nel codice o in chat)
    python3 esporta_schema.py           # elenca team e database disponibili
    python3 esporta_schema.py TEAM_ID DATABASE_ID [cartella_output]

Produce in cartella_output (default: ./schema):
    tabelle.json   risposta grezza dell'API /tables
    schema.json    schema completo, se l'endpoint /schema è disponibile
    schema.md      riepilogo leggibile di tabelle e campi
"""
import json
import os
import sys
import urllib.error
import urllib.request

API = "https://api.ninox.com/v1"


def get(path):
    key = os.environ.get("NINOX_API_KEY")
    if not key:
        sys.exit("Variabile d'ambiente NINOX_API_KEY non impostata.")
    req = urllib.request.Request(API + path, headers={"Authorization": f"Bearer {key}"})
    with urllib.request.urlopen(req, timeout=30) as resp:
        return json.load(resp)


def elenca():
    for team in get("/teams"):
        print(f"Team {team['id']}  {team.get('name', '')}")
        for db in get(f"/teams/{team['id']}/databases"):
            print(f"    Database {db['id']}  {db.get('name', '')}")


def descrivi_campo(campo):
    extra = []
    for chiave in ("refTableId", "refFieldId", "dependent", "required", "fn"):
        if campo.get(chiave) not in (None, False, ""):
            extra.append(f"{chiave}={campo[chiave]}")
    if campo.get("choices"):
        extra.append("scelte: " + ", ".join(c.get("caption", str(c)) for c in campo["choices"]))
    return " | ".join(extra)


def esporta(team_id, db_id, out_dir):
    os.makedirs(out_dir, exist_ok=True)
    base = f"/teams/{team_id}/databases/{db_id}"

    tabelle = get(f"{base}/tables")
    with open(os.path.join(out_dir, "tabelle.json"), "w", encoding="utf-8") as f:
        json.dump(tabelle, f, indent=2, ensure_ascii=False)

    try:
        schema = get(f"{base}/schema")
        with open(os.path.join(out_dir, "schema.json"), "w", encoding="utf-8") as f:
            json.dump(schema, f, indent=2, ensure_ascii=False)
    except urllib.error.HTTPError as e:
        print(f"Endpoint /schema non disponibile ({e.code}): uso solo /tables.")

    nomi = {t["id"]: t.get("name", t["id"]) for t in tabelle}
    righe = ["# Schema Ninox", ""]
    for t in tabelle:
        righe += [f"## {t.get('name')} (id {t['id']})", "",
                  "| Campo | Tipo | Dettagli |", "|---|---|---|"]
        for c in t.get("fields", []):
            dettagli = descrivi_campo(c)
            if c.get("refTableId") in nomi:
                dettagli = f"→ {nomi[c['refTableId']]} " + dettagli
            righe.append(f"| {c.get('name')} | {c.get('type')} | {dettagli} |")
        righe.append("")
    with open(os.path.join(out_dir, "schema.md"), "w", encoding="utf-8") as f:
        f.write("\n".join(righe))

    print(f"Esportate {len(tabelle)} tabelle in {out_dir}/")


if __name__ == "__main__":
    if len(sys.argv) == 1:
        elenca()
    elif len(sys.argv) in (3, 4):
        esporta(sys.argv[1], sys.argv[2], sys.argv[3] if len(sys.argv) == 4 else "schema")
    else:
        sys.exit(__doc__)
