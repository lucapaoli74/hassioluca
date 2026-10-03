"use strict";
// Pannello di Webcam Manager: JavaScript puro, nessuna dipendenza esterna,
// così l'eseguibile funziona anche su PC senza internet.

let cfg = null;          // configurazione corrente (password mascherate)
let logos = [];          // loghi caricati
let timers = [];         // aggiornamenti periodici della pagina attiva
const $ = (s, el = document) => el.querySelector(s);

// ---------- utilità ----------
function esc(s) {
  return String(s ?? "").replace(/[&<>"']/g, c => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
}
function toast(msg, err = false) {
  const t = document.createElement("div");
  t.className = "toast" + (err ? " err" : "");
  t.textContent = msg;
  $("#toasts").appendChild(t);
  setTimeout(() => t.remove(), err ? 7000 : 3500);
}
async function api(method, path, body, raw = false) {
  const opt = { method, headers: {} };
  if (body instanceof Blob || body instanceof ArrayBuffer) opt.body = body;
  else if (body !== undefined) { opt.body = JSON.stringify(body); opt.headers["Content-Type"] = "application/json"; }
  const r = await fetch(path, opt);
  if (!r.ok) {
    let msg = r.statusText;
    try { msg = (await r.json()).error || msg; } catch (_) { }
    throw new Error(msg);
  }
  if (raw) return r;
  const ct = r.headers.get("Content-Type") || "";
  return ct.includes("json") ? r.json() : r.blob();
}
async function busy(btn, fn) {
  const label = btn.textContent;
  btn.disabled = true; btn.textContent = "Attendere…";
  try { return await fn(); }
  catch (e) { toast(e.message, true); }
  finally { btn.disabled = false; btn.textContent = label; }
}
function ago(t) {
  if (!t || t.startsWith("0001")) return "mai";
  const s = Math.round((Date.now() - new Date(t)) / 1000);
  if (s < 60) return s + " s fa";
  if (s < 3600) return Math.round(s / 60) + " min fa";
  if (s < 86400) return Math.round(s / 3600) + " h fa";
  return new Date(t).toLocaleString("it-IT");
}
function fmtBytes(n) { n = n || 0; return n > 1 << 30 ? (n / (1 << 30)).toFixed(1) + " GB" : (n / (1 << 20)).toFixed(0) + " MB"; }
function clone(o) { return JSON.parse(JSON.stringify(o)); }
function slug(s) { return String(s).toLowerCase().normalize("NFD").replace(/[\u0300-\u036f]/g, "").replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "").slice(0, 40) || "item"; }
function uniqueId(base, list) { let id = slug(base), n = 2; while (list.some(x => x.id === id)) id = slug(base) + "-" + n++; return id; }
function every(ms, fn) { fn(); timers.push(setInterval(fn, ms)); }

async function loadConfig() { cfg = await api("GET", "/api/config"); }
async function saveConfig(next) {
  cfg = await api("PUT", "/api/config", next);
  toast("Salvato");
}

// ---------- modulo generico ----------
// fields: [{k, label, type, options, help, wide, show(obj), step, min, max, placeholder}]
function renderForm(obj, fields, onChange) {
  const box = document.createElement("div");
  box.className = "form";
  const draw = () => {
    box.innerHTML = "";
    for (const f of fields) {
      if (f.show && !f.show(obj)) continue;
      const wrap = document.createElement("div");
      wrap.className = "field" + (f.type === "checkbox" ? " check" : "") + (f.wide ? " wide" : "");
      const id = "f" + Math.random().toString(36).slice(2);
      let input;
      if (f.type === "coords") {
        const l = document.createElement("label"); l.textContent = f.label;
        wrap.classList.add("wide"); wrap.append(l, coordsWidget(obj, f.lat || "latitude", f.lon || "longitude", () => onChange && onChange(f.lat || "latitude")));
        if (f.help) { const h = document.createElement("div"); h.className = "help"; h.innerHTML = f.help; wrap.appendChild(h); }
        box.appendChild(wrap);
        continue;
      }
      if (f.type === "select") {
        input = document.createElement("select");
        for (const o of (typeof f.options === "function" ? f.options(obj) : f.options)) {
          const [v, l] = Array.isArray(o) ? o : [o, o];
          const opt = document.createElement("option");
          opt.value = v; opt.textContent = l;
          if (String(obj[f.k] ?? "") === String(v)) opt.selected = true;
          input.appendChild(opt);
        }
      } else if (f.type === "textarea") {
        input = document.createElement("textarea");
        input.value = obj[f.k] ?? "";
      } else {
        input = document.createElement("input");
        input.type = f.type || "text";
        if (f.type === "checkbox") input.checked = !!obj[f.k];
        else input.value = obj[f.k] ?? "";
        for (const a of ["step", "min", "max", "placeholder"]) if (f[a] !== undefined) input[a] = f[a];
        if (f.readonly) input.readOnly = true;
      }
      input.id = id;
      const update = () => {
        let v;
        if (f.type === "checkbox") v = input.checked;
        else if (f.type === "number" || f.type === "range" || f.num) v = input.value === "" ? 0 : Number(input.value);
        else v = input.value;
        obj[f.k] = v;
        if (f.redraw) draw();
        onChange && onChange(f.k);
      };
      input.addEventListener(f.type === "checkbox" || f.type === "select" ? "change" : "input", update);
      if (f.type === "checkbox") {
        const l = document.createElement("label");
        l.appendChild(input); l.append(" " + f.label);
        wrap.appendChild(l);
      } else {
        const l = document.createElement("label");
        l.htmlFor = id; l.textContent = f.label;
        wrap.append(l, input);
      }
      if (f.help) { const h = document.createElement("div"); h.className = "help"; h.innerHTML = f.help; wrap.appendChild(h); }
      box.appendChild(wrap);
    }
  };
  draw();
  box.redraw = draw;
  return box;
}

// ---------- coordinate GPS ----------
// Accetta link di Google Maps (anche brevi), coordinate decimali o in gradi/minuti/secondi.
function parseCoords(text) {
  const t = String(text).trim();
  let m = t.match(/!3d(-?\d+(?:\.\d+)?)!4d(-?\d+(?:\.\d+)?)/) || t.match(/[?&](?:q|query|ll|center|destination)=(-?\d+(?:\.\d+)?),\s*(-?\d+(?:\.\d+)?)/)
    || t.match(/@(-?\d+(?:\.\d+)?),(-?\d+(?:\.\d+)?)/) || t.match(/^\(?\s*(-?\d+(?:[.]\d+)?)\s*[,; ]\s*(-?\d+(?:[.]\d+)?)\s*\)?$/);
  if (m) return [Number(m[1]), Number(m[2])];
  const dms = [...t.matchAll(/(\d+)°\s*(\d+)['′]\s*([\d.]+)["″]?\s*([NSEWO])/gi)];
  if (dms.length === 2) {
    const v = d => (Number(d[1]) + Number(d[2]) / 60 + Number(d[3]) / 3600) * (/[SWO]/i.test(d[4]) ? -1 : 1);
    return [v(dms[0]), v(dms[1])];
  }
  return null;
}
function coordsWidget(obj, latK, lonK, onChange) {
  const el = document.createElement("div");
  el.innerHTML = `<div class="toolbar" style="margin:0">
      <input data-lat type="number" step="any" placeholder="Latitudine" style="width:140px"><input data-lon type="number" step="any" placeholder="Longitudine" style="width:140px">
      <input data-paste placeholder="📋 Incolla qui un link di Google Maps o le coordinate" style="flex:1;min-width:220px">
      <button type="button" data-map>🗺️ Scegli sulla mappa</button><a class="btn" data-gm target="_blank" rel="noopener">Verifica su Google Maps ↗</a></div>`;
  const [la, lo, paste] = [el.querySelector("[data-lat]"), el.querySelector("[data-lon]"), el.querySelector("[data-paste]")];
  const sync = () => {
    la.value = obj[latK] || ""; lo.value = obj[lonK] || "";
    el.querySelector("[data-gm]").href = `https://www.google.com/maps/search/?api=1&query=${obj[latK] || 0},${obj[lonK] || 0}`;
  };
  const set = (a, b) => { obj[latK] = Math.round(a * 1e6) / 1e6; obj[lonK] = Math.round(b * 1e6) / 1e6; sync(); onChange && onChange(); };
  la.oninput = () => { obj[latK] = Number(la.value); sync(); onChange && onChange(); };
  lo.oninput = () => { obj[lonK] = Number(lo.value); sync(); onChange && onChange(); };
  paste.onchange = paste.onpaste = () => setTimeout(async () => {
    let v = paste.value.trim(); if (!v) return;
    let c = parseCoords(v);
    if (!c && /goo\.gl|google\./.test(v)) {
      try { v = (await api("POST", "/api/resolve-link", { URL: v })).url; c = parseCoords(v); } catch (e) { toast(e.message, true); return; }
    }
    if (!c) { toast("Non trovo coordinate: in Google Maps fai clic destro sul punto e copia le coordinate, oppure copia il link.", true); return; }
    set(c[0], c[1]); paste.value = ""; toast(`Coordinate impostate: ${c[0].toFixed(5)}, ${c[1].toFixed(5)}`);
  }, 0);
  el.querySelector("[data-map]").onclick = () => mapPicker(obj[latK], obj[lonK], set);
  sync();
  return el;
}
function loadLeaflet() {
  if (window.L) return Promise.resolve();
  return new Promise((ok, ko) => {
    const css = document.createElement("link"); css.rel = "stylesheet"; css.href = "https://cdnjs.cloudflare.com/ajax/libs/leaflet/1.9.4/leaflet.min.css"; document.head.appendChild(css);
    const js = document.createElement("script"); js.src = "https://cdnjs.cloudflare.com/ajax/libs/leaflet/1.9.4/leaflet.min.js";
    js.onload = ok; js.onerror = () => ko(new Error("Mappa non disponibile senza internet: incolla le coordinate da Google Maps."));
    document.head.appendChild(js);
  });
}
async function mapPicker(lat, lon, set) {
  try { await loadLeaflet(); } catch (e) { toast(e.message, true); return; }
  openModal(`<h2 style="margin-top:0">Scegli il punto sulla mappa</h2>
    <div class="toolbar"><input id="mp-q" placeholder="Cerca località (es. Sant'Anna Pelago)" style="flex:1"><button id="mp-go">Cerca</button><span id="mp-c" class="help"></span></div>
    <div id="mp-map" style="height:60vh;border-radius:8px"></div>
    <div class="toolbar" style="margin-top:10px"><span class="help">Clic sulla mappa o trascina il segnaposto.</span><span class="spacer"></span><button class="primary" id="mp-ok">Usa questo punto</button></div>`);
  const start = lat || lon ? [lat, lon] : [44.2, 10.58];
  const map = L.map("mp-map").setView(start, lat || lon ? 14 : 8);
  L.tileLayer("https://tile.openstreetmap.org/{z}/{x}/{y}.png", { maxZoom: 19, attribution: "© OpenStreetMap" }).addTo(map);
  const mk = L.marker(start, { draggable: true }).addTo(map);
  const show = () => { const p = mk.getLatLng(); $("#mp-c").textContent = `${p.lat.toFixed(5)}, ${p.lng.toFixed(5)}`; };
  map.on("click", e => { mk.setLatLng(e.latlng); show(); });
  mk.on("dragend", show); show();
  const search = async () => {
    const q = $("#mp-q").value.trim(); if (!q) return;
    const r = await fetch("https://geocoding-api.open-meteo.com/v1/search?count=1&language=it&name=" + encodeURIComponent(q)).then(r => r.json()).catch(() => ({}));
    const g = r.results && r.results[0];
    if (!g) { toast("Località non trovata", true); return; }
    mk.setLatLng([g.latitude, g.longitude]); map.setView([g.latitude, g.longitude], 14); show();
  };
  $("#mp-go").onclick = search; $("#mp-q").onkeydown = e => { if (e.key === "Enter") search(); };
  $("#mp-ok").onclick = () => { const p = mk.getLatLng(); set(p.lat, p.lng); closeModal(); };
}

// ---------- modale / live ----------
function openModal(html) {
  $("#modal-body").innerHTML = "";
  if (typeof html === "string") $("#modal-body").innerHTML = html; else $("#modal-body").appendChild(html);
  $("#modal").classList.remove("hidden");
}
function closeModal() {
  // interrompe eventuali stream live (img con src MJPEG)
  $("#modal-body").querySelectorAll("img").forEach(i => i.src = "");
  $("#modal").classList.add("hidden");
  $("#modal-body").innerHTML = "";
}
$("#modal").addEventListener("click", e => { if (e.target.id === "modal" || e.target.classList.contains("modal-close")) closeModal(); });
document.addEventListener("keydown", e => { if (e.key === "Escape") closeModal(); });

function showLive(title, src, extra = "") {
  openModal(`<h2 style="margin-top:0">🔴 ${esc(title)}</h2>
    <img class="live-big" src="${esc(src)}" alt="" onerror="this.replaceWith(Object.assign(document.createElement('p'),{className:'err-text',textContent:'Flusso non disponibile: controlla URL, credenziali e (per RTSP) la presenza di ffmpeg.'}))">
    <div class="toolbar" style="margin-top:10px">${extra}</div>`);
}
async function liveSession(cam) {
  const { token } = await api("POST", "/api/live-session", cam);
  return "/live/session/" + token;
}

// ---------- navigazione ----------
const pages = {};
async function route() {
  timers.forEach(clearInterval); timers = [];
  document.querySelectorAll("#page img").forEach(i => i.src = ""); // chiude gli stream
  const [name, arg] = (location.hash.slice(1) || "stato").split("/");
  document.querySelectorAll("#menu a").forEach(a => a.classList.toggle("active", a.dataset.page === name));
  $("#menu").classList.remove("open");
  const page = $("#page");
  page.innerHTML = "";
  try {
    if (!cfg) await loadConfig();
    await (pages[name] || pages.stato)(page, arg && decodeURIComponent(arg));
  } catch (e) {
    page.innerHTML = `<div class="card err-text">${esc(e.message)}</div>`;
  }
}
window.addEventListener("hashchange", route);
$("#menu-toggle").onclick = () => $("#menu").classList.toggle("open");

// ======================================================================
// STATO
// ======================================================================
pages.stato = async (page) => {
  page.innerHTML = `<h1>Stato</h1>
    <div id="st-empty"></div>
    <div class="grid" id="st-cams"></div>
    <h2>Pubblicazioni</h2>
    <div class="card"><table><thead><tr><th>Sito</th><th>File</th><th>Webcam</th><th>Ultimo invio</th><th>Esito</th></tr></thead><tbody id="st-pubs"></tbody></table></div>
    <h2>Dati live</h2><div class="card" id="st-data"></div>`;
  if (!cfg.location.name) {
    $("#st-empty").insertAdjacentHTML("beforebegin", `<div class="card" id="loc-wiz"><h3 style="margin-top:0">Benvenuto! Come si chiama questa località?</h3>
      <p class="help">Il nome e l'altitudine compaiono nelle scritte sulle immagini ({location}, {altitude}), nel titolo dello storico e nelle email.</p>
      <div class="toolbar" style="margin:0"><input id="lw-name" placeholder="Es. Camping Coggiolo Sant'Anna Pelago (MO)" style="flex:2;min-width:240px">
      <input id="lw-alt" type="number" placeholder="Altitudine m s.l.m." style="flex:1;min-width:150px"><button class="primary" id="lw-save">Salva</button></div></div>`);
    $("#lw-save").onclick = e => busy(e.target, async () => {
      const next = clone(cfg); next.location.name = $("#lw-name").value.trim(); next.location.altitude = Number($("#lw-alt").value) || 0;
      if (!next.location.name) throw new Error("Scrivi il nome della località");
      await saveConfig(next); $("#loc-wiz").remove(); setVersion();
    });
  }
  if (!cfg.cameras.length) {
    $("#st-empty").innerHTML = `<div class="card">Nessuna webcam configurata. Parti da <a href="#ricerca">Ricerca telecamere</a> oppure <a href="#webcam/nuova">aggiungine una a mano</a>.</div>`;
  }
  const grid = $("#st-cams");
  for (const c of cfg.cameras) {
    const el = document.createElement("div");
    el.className = "card cam-card";
    el.innerHTML = `<img class="cam-img" alt="">
      <h3><span>${esc(c.name || c.id)}</span><span class="badge" data-badge></span></h3>
      <div class="help" data-info></div><div class="err-text" data-err></div>
      <div class="toolbar" style="margin:10px 0 0">
        <button class="small" data-act="cap">📸 Cattura ora</button>
        <button class="small" data-act="live">🔴 Live</button>
        <a class="btn small" href="#webcam/${encodeURIComponent(c.id)}">✏️ Modifica</a>
      </div>`;
    el.querySelector("[data-act=cap]").onclick = e => busy(e.target, async () => { await api("POST", `/api/cameras/${c.id}/capture`); toast("Cattura richiesta"); });
    el.querySelector("[data-act=live]").onclick = () => showLive(c.name || c.id, "/live/" + encodeURIComponent(c.id));
    el.dataset.id = c.id;
    grid.appendChild(el);
  }
  every(5000, async () => {
    const st = await api("GET", "/api/status");
    for (const el of grid.children) {
      const c = cfg.cameras.find(x => x.id === el.dataset.id);
      const s = st.cameras[c.id] || {};
      const badge = el.querySelector("[data-badge]");
      if (!c.enabled) { badge.className = "badge"; badge.textContent = "disattivata"; }
      else if (s.out_of_hours) { badge.className = "badge warn"; badge.textContent = "fuori orario"; }
      else if (s.last_error) { badge.className = "badge err"; badge.textContent = "errore"; }
      else if (s.last_capture) { badge.className = "badge ok"; badge.textContent = "ok"; }
      else { badge.className = "badge"; badge.textContent = "in attesa"; }
      el.querySelector("[data-info]").textContent = `Ultima cattura: ${ago(s.last_capture)} · ogni ${c.interval_seconds}s` + (c.archive_minutes ? ` · storico: ${s.archive_count || 0} immagini, ${fmtBytes(s.archive_bytes)}` : "");
      el.querySelector("[data-err]").textContent = s.last_error || "";
      const img = el.querySelector("img");
      if (s.last_capture) img.src = `/snapshot/${encodeURIComponent(c.id)}.jpg?t=${Date.parse(s.last_capture)}`;
    }
    const names = Object.fromEntries(cfg.sites.map(s => [s.id, s.name || s.id]));
    $("#st-pubs").innerHTML = st.pubs.length ? st.pubs.map(p => `<tr><td>${esc(names[p.site_id] || p.site_id)}</td><td>${esc(p.filename)}</td><td>${esc(p.camera_id)}</td>
      <td>${ago(p.last_upload)}</td><td>${p.last_error ? `<span class="badge err">errore</span> <span class="err-text">${esc(p.last_error)}</span>` : '<span class="badge ok">ok</span>'}</td></tr>`).join("")
      : `<tr><td colspan="5" class="help">Nessun invio ancora effettuato.</td></tr>`;
    for (const [id, s] of Object.entries(st.sites)) {
      if (s.history_error) $("#st-pubs").insertAdjacentHTML("beforeend", `<tr><td>${esc(names[id] || id)}</td><td>storico</td><td></td><td>${ago(s.last_history_sync)}</td><td><span class="badge err">errore</span> <span class="err-text">${esc(s.history_error)}</span></td></tr>`);
    }
    const ds = cfg.data_sources;
    $("#st-data").innerHTML = ds.length ? `<table><tr><th>Dato</th><th>Valore</th><th>Aggiornato</th></tr>${ds.map(d => {
      const v = st.data[d.id] || {};
      return `<tr><td>${esc(d.name || d.id)} <code>{data:${esc(d.id)}}</code></td><td><b>${esc(v.value || "—")}</b> ${v.error ? `<span class="err-text">${esc(v.error)}</span>` : ""}</td><td>${ago(v.updated_at)}</td></tr>`;
    }).join("")}</table>` : `<span class="help">Nessun dato live. Aggiungili in <a href="#dati">Dati live</a>.</span>`;
  });
};

// ======================================================================
// VISTA LIVE
// ======================================================================
pages.live = async (page) => {
  page.innerHTML = `<h1>Vista live</h1>
    <div class="toolbar"><span class="help">Clic su una webcam per ingrandirla. Il flusso resta aperto solo finché la pagina è visibile.</span><span class="spacer"></span>
    <label class="help">Colonne <select id="lv-cols" style="width:auto"><option>1</option><option selected>2</option><option>3</option><option>4</option></select></label></div>
    <div class="grid" id="lv-grid"></div>`;
  const cams = cfg.cameras.filter(c => c.enabled);
  if (!cams.length) { $("#lv-grid").outerHTML = `<div class="card">Nessuna webcam attiva. Puoi vedere in diretta anche le telecamere trovate in <a href="#ricerca">Ricerca telecamere</a>.</div>`; return; }
  const grid = $("#lv-grid");
  const setCols = () => grid.style.gridTemplateColumns = `repeat(${$("#lv-cols").value}, minmax(0, 1fr))`;
  $("#lv-cols").onchange = setCols; setCols();
  for (const c of cams) {
    const t = document.createElement("div");
    t.className = "live-tile";
    t.innerHTML = `<img class="cam-img" src="/live/${encodeURIComponent(c.id)}" alt=""><span class="label"><span class="dot"></span>${esc(c.name || c.id)}</span>`;
    t.onclick = () => showLive(c.name || c.id, "/live/" + encodeURIComponent(c.id));
    grid.appendChild(t);
  }
};

// ======================================================================
// RICERCA TELECAMERE
// ======================================================================
let lastFound = [];
pages.ricerca = async (page) => {
  page.innerHTML = `<h1>Ricerca telecamere</h1>
    <div class="card">
      <p class="help" style="margin-top:0">Cerca le telecamere IP nella rete locale con <b>ONVIF</b> (WS-Discovery) e con una scansione delle porte tipiche (RTSP 554, web, Hikvision, Dahua…). Dura circa 15 secondi.</p>
      <div class="toolbar" style="margin:0"><button class="primary" id="rc-go">🔎 Avvia ricerca</button>
      <span class="spacer"></span><span class="help">Credenziali da provare sulle telecamere:</span>
      <input id="rc-user" placeholder="utente" style="width:130px" value="admin"><input id="rc-pass" type="password" placeholder="password" style="width:150px"></div>
    </div>
    <div id="rc-res"></div>
    <div class="card"><h3 style="margin-top:0">Hai già l'indirizzo?</h3>
      <div class="toolbar" style="margin:0"><select id="rc-src" style="width:130px"><option value="rtsp">RTSP</option><option value="snapshot">Snapshot HTTP</option><option value="mjpeg">MJPEG</option></select>
      <input id="rc-url" placeholder="rtsp://192.168.1.64:554/... oppure http://.../snapshot.jpg" style="flex:1;min-width:260px">
      <button id="rc-live">🔴 Vedi live</button><button id="rc-add">➕ Aggiungi</button></div></div>`;
  const creds = () => ({ username: $("#rc-user").value, password: $("#rc-pass").value });
  $("#rc-live").onclick = e => busy(e.target, async () => showLive($("#rc-url").value, await liveSession({ source: $("#rc-src").value, url: $("#rc-url").value, ...creds() })));
  $("#rc-add").onclick = () => addFromDiscovery({ source: $("#rc-src").value, url: $("#rc-url").value, ...creds() }, "Webcam");
  $("#rc-go").onclick = e => busy(e.target, async () => {
    $("#rc-res").innerHTML = `<div class="card help">Ricerca in corso…</div>`;
    lastFound = await api("POST", "/api/discover");
    drawFound();
  });
  if (lastFound.length) drawFound();

  function drawFound() {
    const res = $("#rc-res");
    if (!lastFound.length) { res.innerHTML = `<div class="card">Nessuna telecamera trovata. Verifica che il PC sia nella stessa rete delle telecamere, oppure inserisci l'indirizzo a mano qui sotto.</div>`; return; }
    res.innerHTML = `<h2>${lastFound.length} dispositivi trovati</h2>`;
    for (const d of lastFound) {
      const card = document.createElement("div");
      card.className = "card";
      card.innerHTML = `<div class="toolbar" style="margin:0 0 6px"><b style="font-size:16px">${esc(d.ip)}</b>
        ${d.brand ? `<span class="badge">${esc(d.brand)}</span>` : ""} ${d.onvif ? `<span class="badge ok">ONVIF</span>` : ""}
        ${d.configured ? `<span class="badge warn">già configurata: ${esc(d.configured)}</span>` : ""}
        <span class="help">${esc([d.name, d.hardware].filter(Boolean).join(" · "))}</span><span class="spacer"></span>
        <span class="help">porte ${esc((d.ports || []).join(", "))}</span>
        ${d.onvif ? `<button class="small" data-onvif>Leggi URL via ONVIF</button>` : ""}
        <a class="btn small" href="http://${esc(d.ip)}" target="_blank" rel="noopener">Pagina web ↗</a></div>
        ${d.server ? `<div class="help">${esc(d.server)}</div>` : ""}
        ${d.brand === "EZVIZ" || d.brand === "Hikvision" ? `<div class="help">💡 EZVIZ: utente <b>admin</b>, password = <b>codice di verifica</b> di 6 lettere sull'etichetta della telecamera. Se non risponde, nell'app EZVIZ attiva RTSP / “Visione live in LAN”.</div>` : ""}
        <table data-urls><tbody></tbody></table>`;
      const tbody = card.querySelector("tbody");
      const addRow = (label, source, url) => {
        const tr = document.createElement("tr");
        tr.innerHTML = `<td>${esc(label)}</td><td><span class="badge">${esc(source)}</span></td><td style="word-break:break-all"><code>${esc(url)}</code></td>
          <td style="white-space:nowrap"><button class="small" data-l>🔴 Live</button> <button class="small" data-a>➕ Aggiungi / pubblica</button></td>`;
        tr.querySelector("[data-l]").onclick = e => busy(e.target, async () => showLive(`${d.ip} · ${label}`, await liveSession({ source, url, ...creds() })));
        tr.querySelector("[data-a]").onclick = () => addFromDiscovery({ source, url, ...creds() }, [d.brand, d.ip].filter(Boolean).join(" "));
        tbody.appendChild(tr);
      };
      (d.suggestions || []).forEach(s => addRow(s.label, s.source, s.url));
      const ob = card.querySelector("[data-onvif]");
      if (ob) ob.onclick = e => busy(e.target, async () => {
        const info = await api("POST", "/api/onvif", { XAddr: d.xaddr, ...{ Username: creds().username, Password: creds().password } });
        tbody.innerHTML = "";
        card.querySelector(".help").textContent = `${info.manufacturer} ${info.model} · firmware ${info.firmware}`;
        for (const p of info.profiles) {
          const res = p.width ? ` ${p.width}×${p.height}` : "";
          if (p.stream_uri) addRow(`${p.name}${res}`, "rtsp", p.stream_uri);
          if (p.snapshot_uri) addRow(`${p.name} snapshot`, "snapshot", p.snapshot_uri);
        }
        if (!info.profiles.length) toast("Nessun profilo video restituito dalla telecamera", true);
      });
      if (!tbody.children.length && !d.onvif) tbody.innerHTML = `<tr><td class="help">Nessun URL noto: inseriscilo a mano qui sotto.</td></tr>`;
      res.appendChild(card);
    }
  }
};
function addFromDiscovery(cam, name) {
  sessionStorage.setItem("newCamera", JSON.stringify({ ...cam, name }));
  location.hash = "#webcam/nuova";
}

// ======================================================================
// WEBCAM
// ======================================================================
const cameraFields = [
  { k: "name", label: "Nome", placeholder: "Es. Porto di Lerici" },
  { k: "id", label: "ID (nome breve, senza spazi)", help: "Usato nei nomi dei file e negli indirizzi." },
  { k: "enabled", label: "Attiva", type: "checkbox" },
  { k: "public", label: "Pubblica su /public/ID.jpg (senza password)", type: "checkbox" },
  { k: "source", label: "Tipo di sorgente", type: "select", options: [["snapshot", "Snapshot HTTP (JPEG)"], ["mjpeg", "Stream MJPEG"], ["rtsp", "Stream RTSP (richiede ffmpeg)"]] },
  { k: "url", label: "URL", wide: true },
  { k: "username", label: "Utente telecamera" },
  { k: "password", label: "Password telecamera", type: "password" },
  { k: "interval_seconds", label: "Cattura ogni (secondi)", type: "number", min: 5, help: "La periodicità di pubblicazione si imposta per ogni sito." },
  { k: "active_from", label: "Attiva dalle", type: "time", help: "Vuoto = sempre. Es. 06:00–21:30" },
  { k: "active_to", label: "Attiva fino alle", type: "time" },
  { k: "archive_minutes", label: "Storico: un'immagine ogni", type: "select", num: true, options: [[0, "Storico disattivato"], [10, "10 minuti"], [15, "15 minuti"], [20, "20 minuti"], [30, "30 minuti"], [60, "1 ora"], [120, "2 ore"], [180, "3 ore"]] },
  { k: "retention_mode", label: "Conservazione storico locale", type: "select", redraw: true, show: c => c.archive_minutes > 0, options: [["forever", "Per sempre"], ["days", "Per un periodo"], ["size", "Fino a uno spazio massimo"]] },
  { k: "retention_days", label: "Conserva per", type: "select", num: true, show: c => c.archive_minutes > 0 && c.retention_mode === "days", options: [[7, "1 settimana"], [30, "1 mese"], [60, "2 mesi"], [90, "3 mesi"], [180, "6 mesi"], [365, "1 anno"], [730, "2 anni"], [1825, "5 anni"]] },
  { k: "retention_gb", label: "Spazio massimo (GB)", type: "number", step: 0.5, min: 0.1, show: c => c.archive_minutes > 0 && c.retention_mode === "size", help: "Superato il limite vengono cancellate le immagini più vecchie." },
  { k: "timelapse_minutes", label: "Timelapse nel mini-sito", type: "select", num: true, redraw: true, options: [[0, "Disattivato"], [2, "un fotogramma ogni 2 min"], [5, "ogni 5 minuti"], [10, "ogni 10 minuti"], [15, "ogni 15 minuti"]] },
  { k: "timelapse_hours", label: "Durata timelapse", type: "select", num: true, show: c => c.timelapse_minutes > 0, options: [[3, "ultime 3 ore"], [6, "ultime 6 ore"], [12, "ultime 12 ore"], [24, "ultime 24 ore"]] },
  { k: "max_width", label: "Larghezza massima (px)", type: "number", help: "0 = risoluzione originale. Es. 1920 o 1280 per siti più leggeri." },
  { k: "quality", label: "Qualità JPEG (1-100)", type: "number", min: 1, max: 100 },
];
function newCamera() {
  return { id: "", name: "", enabled: true, source: "snapshot", url: "", username: "", password: "", interval_seconds: 120, max_width: 1920, quality: 85,
    public: false, active_from: "", active_to: "", archive_minutes: 60, retention_days: 0, retention_mb: 0, timelapse_minutes: 5, timelapse_hours: 6,
    overlays: [{ type: "text", enabled: true, text: (cfg.location.name ? "{location} · {altitude} · " : "{name} · ") + "{date} {time}", anchor: "bottom-left", offset_x: 0, offset_y: 0, size: 3.5, color: "#ffffff", bold: true, shadow: true, background: "#000000", bg_opacity: 0.45, opacity: 1, full_width: true }] };
}
pages.webcam = async (page, arg) => {
  if (arg) return editCamera(page, arg);
  page.innerHTML = `<h1>Webcam</h1><div class="toolbar"><a class="btn primary" href="#webcam/nuova">➕ Nuova webcam</a><a class="btn" href="#ricerca">🔎 Cerca in rete</a></div><div class="card" id="wc-list"></div>`;
  const list = $("#wc-list");
  if (!cfg.cameras.length) { list.innerHTML = `<span class="help">Nessuna webcam.</span>`; return; }
  for (const c of cfg.cameras) {
    const sites = cfg.sites.filter(s => s.publications.some(p => p.camera_id === c.id)).map(s => s.name || s.id);
    const it = document.createElement("div");
    it.className = "list-item";
    it.innerHTML = `<img src="/snapshot/${encodeURIComponent(c.id)}.jpg?t=${Date.now()}" alt="" onerror="this.style.visibility='hidden'">
      <div class="grow"><b>${esc(c.name || c.id)}</b> ${c.enabled ? "" : '<span class="badge">disattivata</span>'}<div class="help">${esc(c.source)} · ogni ${c.interval_seconds}s · ${c.overlays.filter(o => o.enabled).length} sovrimpressioni · pubblicata su: ${esc(sites.join(", ") || "nessun sito")}</div></div>
      <a class="btn small" href="#webcam/${encodeURIComponent(c.id)}">Modifica</a>
      <a class="btn small" href="#sovrimpressioni/${encodeURIComponent(c.id)}">Sovrimpressioni</a>`;
    list.appendChild(it);
  }
};
async function editCamera(page, id) {
  const isNew = id === "nuova";
  let cam;
  if (isNew) {
    cam = newCamera();
    const pre = sessionStorage.getItem("newCamera");
    if (pre) { Object.assign(cam, JSON.parse(pre)); sessionStorage.removeItem("newCamera"); }
    if (cam.name) cam.id = uniqueId(cam.name, cfg.cameras);
  } else {
    cam = clone(cfg.cameras.find(c => c.id === id) || {});
    if (!cam.id) { page.innerHTML = `<div class="card">Webcam non trovata.</div>`; return; }
  }
  const origId = cam.id;
  cam.retention_mode = cam.retention_mb > 0 ? "size" : cam.retention_days > 0 ? "days" : "forever";
  cam.retention_gb = cam.retention_mb > 0 ? Math.round(cam.retention_mb / 102.4) / 10 : 5;
  if (!cam.retention_days) cam.retention_days = 365;
  page.innerHTML = `<h1>${isNew ? "Nuova webcam" : "Webcam: " + esc(cam.name || cam.id)}</h1>
    <div class="editor"><div><div class="card" id="cm-form"></div>
      <div class="card"><h3 style="margin-top:0">Pubblicazione</h3><div id="cm-pubs"></div></div>
      <div class="toolbar"><button class="primary" id="cm-save">💾 Salva</button><a class="btn" href="#webcam">Annulla</a><span class="spacer"></span>${isNew ? "" : '<button class="danger" id="cm-del">Elimina</button>'}</div></div>
    <div class="preview card"><h3 style="margin-top:0">Prova</h3><img class="cam-img" id="cm-img" alt=""><div id="cm-msg" class="help" style="margin-top:6px">Premi “Prova” per verificare URL e credenziali.</div>
      <div class="toolbar" style="margin-top:10px"><button id="cm-test">📸 Prova cattura</button><button id="cm-live">🔴 Live</button>${isNew ? "" : `<a class="btn" href="#sovrimpressioni/${encodeURIComponent(cam.id)}">🖌️ Sovrimpressioni</a>`}</div></div></div>`;
  $("#cm-form").appendChild(renderForm(cam, cameraFields, k => {
    if (k === "name" && isNew) { cam.id = uniqueId(cam.name, cfg.cameras); $("#cm-form .form").redraw(); }
    // il tipo di sorgente segue l'indirizzo: rtsp:// → RTSP
    if (k === "url") {
      const u = cam.url.trim().toLowerCase();
      const src = u.startsWith("rtsp") ? "rtsp" : (u.startsWith("http") && cam.source === "rtsp") ? "snapshot" : cam.source;
      if (src !== cam.source) { cam.source = src; const sel = $("#cm-form select"); if (sel) sel.value = src; }
    }
  }));

  // pubblicazione su siti: una riga per sito con nome file e periodicità
  const pubBox = $("#cm-pubs");
  const pubState = cfg.sites.map(s => {
    const p = s.publications.find(p => p.camera_id === origId);
    return { site: s.id, on: !!p, filename: p ? p.filename : "", interval: p ? p.interval_seconds : 120 };
  });
  if (!cfg.sites.length) pubBox.innerHTML = `<span class="help">Nessun sito configurato: aggiungili in <a href="#siti">Siti FTP</a>.</span>`;
  else {
    pubBox.innerHTML = `<table><thead><tr><th></th><th>Sito</th><th>Nome file</th><th>Pubblica</th></tr></thead><tbody></tbody></table>`;
    pubState.forEach((p, i) => {
      const s = cfg.sites[i];
      const tr = document.createElement("tr");
      tr.innerHTML = `<td><input type="checkbox" ${p.on ? "checked" : ""}></td><td>${esc(s.name || s.id)}</td>
        <td><input value="${esc(p.filename)}" placeholder="${esc((cam.id || "webcam") + ".jpg")}"></td><td>${intervalSelect(p.interval)}</td>`;
      const [chk, fn, sel] = tr.querySelectorAll("input, select");
      chk.onchange = () => p.on = chk.checked;
      fn.oninput = () => p.filename = fn.value;
      sel.onchange = () => p.interval = Number(sel.value);
      pubBox.querySelector("tbody").appendChild(tr);
    });
  }

  $("#cm-test").onclick = e => busy(e.target, async () => {
    $("#cm-msg").textContent = "Cattura in corso…";
    try {
      const blob = await api("POST", "/api/test-camera", cam);
      $("#cm-img").src = URL.createObjectURL(blob);
      $("#cm-msg").innerHTML = `<span class="ok-text">✔ Immagine ricevuta (${Math.round(blob.size / 1024)} KB)</span>`;
    } catch (err) { $("#cm-msg").innerHTML = `<span class="err-text">✖ ${esc(err.message)}</span>`; }
  });
  $("#cm-live").onclick = e => busy(e.target, async () => showLive(cam.name || cam.url, await liveSession(cam)));
  $("#cm-save").onclick = e => busy(e.target, async () => {
    const mode = cam.retention_mode;
    if (mode !== "days") cam.retention_days = 0;
    cam.retention_mb = mode === "size" ? Math.round(cam.retention_gb * 1024) : 0;
    delete cam.retention_mode; delete cam.retention_gb;
    const next = clone(cfg);
    const idx = next.cameras.findIndex(c => c.id === origId);
    if (isNew) next.cameras.push(cam); else next.cameras[idx] = cam;
    next.sites.forEach((s, i) => {
      const st = pubState[i];
      s.publications = s.publications.filter(p => p.camera_id !== origId);
      if (st && st.on) s.publications.push({ camera_id: cam.id, filename: st.filename || cam.id + ".jpg", interval_seconds: st.interval });
    });
    await saveConfig(next);
    location.hash = "#webcam";
  });
  const del = $("#cm-del");
  if (del) del.onclick = e => {
    if (!confirm(`Eliminare la webcam "${cam.name || cam.id}"? Lo storico già salvato resta su disco.`)) return;
    busy(e.target, async () => {
      const next = clone(cfg);
      next.cameras = next.cameras.filter(c => c.id !== origId);
      next.sites.forEach(s => s.publications = s.publications.filter(p => p.camera_id !== origId));
      await saveConfig(next);
      location.hash = "#webcam";
    });
  };
}
function intervalSelect(v) {
  const opts = [[0, "a ogni cattura"], [60, "ogni minuto"], [120, "ogni 2 minuti"], [300, "ogni 5 minuti"], [600, "ogni 10 minuti"], [900, "ogni 15 minuti"], [1800, "ogni 30 minuti"], [3600, "ogni ora"]];
  if (!opts.some(o => o[0] === v)) opts.push([v, `ogni ${v} s`]);
  return `<select>${opts.map(([s, l]) => `<option value="${s}" ${s === v ? "selected" : ""}>${l}</option>`).join("")}</select>`;
}

// ======================================================================
// SOVRIMPRESSIONI
// ======================================================================
const anchorsList = [["top-left", "↖"], ["top-center", "↑"], ["top-right", "↗"], ["middle-left", "←"], ["center", "•"], ["middle-right", "→"], ["bottom-left", "↙"], ["bottom-center", "↓"], ["bottom-right", "↘"]];
pages.sovrimpressioni = async (page, arg) => {
  logos = await api("GET", "/api/logos");
  if (!cfg.cameras.length) { page.innerHTML = `<h1>Sovrimpressioni</h1><div class="card">Aggiungi prima una webcam.</div>`; return; }
  const camId = arg && cfg.cameras.some(c => c.id === arg) ? arg : cfg.cameras[0].id;
  const cam = clone(cfg.cameras.find(c => c.id === camId));
  page.innerHTML = `<h1>Sovrimpressioni</h1>
    <div class="toolbar"><label>Webcam <select id="ov-cam" style="width:auto">${cfg.cameras.map(c => `<option value="${esc(c.id)}" ${c.id === camId ? "selected" : ""}>${esc(c.name || c.id)}</option>`).join("")}</select></label>
      <span class="spacer"></span><button id="ov-copy">Copia da altra webcam…</button><button class="primary" id="ov-save">💾 Salva</button></div>
    <div class="editor"><div>
      <div class="toolbar"><button id="ov-add-text">➕ Scritta</button><button id="ov-add-time">🕒 Data e ora</button><button id="ov-add-data">🌡️ Dato live</button><button id="ov-add-logo">🖼️ Logo</button><button id="ov-add-fc">⛅ Previsioni</button></div>
      <div id="ov-layers"></div>
      <div class="card"><h3 style="margin-top:0">Libreria loghi</h3><p class="help">PNG con sfondo trasparente per il miglior risultato.</p>
        <div class="logos" id="ov-logos"></div>
        <div class="toolbar" style="margin:10px 0 0"><input type="file" id="ov-file" accept=".png,.jpg,.jpeg" style="width:auto"><button id="ov-upload">Carica logo</button></div></div>
    </div>
    <div class="preview card"><h3 style="margin-top:0">Anteprima <span class="help" style="font-weight:400">— trascina gli elementi con il mouse</span></h3>
      <div class="ov-stage" id="ov-stage"><img id="ov-prev" alt=""><div id="ov-boxes"></div></div>
      <p class="help">Clic per selezionare, trascina per spostare, maniglia ◢ per ridimensionare. Applicata all'ultima immagine della webcam (o a un'immagine di prova) con i valori attuali dei dati live.</p>
      <div class="help"><b>Segnaposto</b> (clic per inserire nella scritta selezionata):</div><div class="chips" id="ov-chips"></div></div></div>`;
  $("#ov-cam").onchange = () => location.hash = "#sovrimpressioni/" + encodeURIComponent($("#ov-cam").value);

  let focused = null; // textarea della scritta attiva
  const chips = [["{name}", "nome webcam"], ["{date}", "03/10/2026"], ["{time}", "14:05"], ["{seconds}", "14:05:09"], ["{weekday}", "sabato"], ["{longdate}", "sabato 3 ottobre 2026"], ["{month}", "ottobre"], ["{year}", "2026"], ["{datetime:2006-01-02 15:04}", "formato libero"]]
    .concat([["{location}", cfg.location.name || "nome località"], ["{altitude}", cfg.location.altitude ? cfg.location.altitude + " m s.l.m." : "altitudine"], ["{site_url}", "indirizzo del sito"], ["{history_url}", "indirizzo dello storico"]])
    .concat(cfg.data_sources.map(d => [`{data:${d.id}}`, d.name || d.id]));
  $("#ov-chips").innerHTML = chips.map(([c, l]) => `<span class="chip" title="${esc(l)}" data-c="${esc(c)}">${esc(c)} <span class="help">${esc(l)}</span></span>`).join("");
  $("#ov-chips").onclick = e => {
    const c = e.target.closest(".chip"); if (!c) return;
    if (!focused) { toast("Seleziona prima il testo di una scritta"); return; }
    const ta = focused, pos = ta.selectionStart ?? ta.value.length;
    ta.value = ta.value.slice(0, pos) + c.dataset.c + ta.value.slice(ta.selectionEnd ?? pos);
    ta.dispatchEvent(new Event("input")); ta.focus();
  };

  let tmr = null, seq = 0, selected = -1, last = null;
  const preview = () => { clearTimeout(tmr); tmr = setTimeout(async () => {
    const my = ++seq;
    try {
      const r = await api("POST", "/api/overlay-preview", cam);
      if (my !== seq || !$("#ov-prev")) return; // risposta superata da una più recente
      last = r; $("#ov-prev").src = r.image; drawBoxes();
    } catch (e) { toast(e.message, true); }
  }, 250); };
  // riquadri trascinabili sopra l'anteprima
  function drawBoxes() {
    const host = $("#ov-boxes"); if (!host || !last) return;
    host.innerHTML = "";
    const img = $("#ov-prev"), sc = img.clientWidth / last.width;
    last.boxes.forEach((b, i) => {
      if (!b.W || !cam.overlays[i] || !cam.overlays[i].enabled) return;
      const d = document.createElement("div");
      d.className = "ov-box" + (i === selected ? " sel" : "");
      Object.assign(d.style, { left: b.X * sc + "px", top: b.Y * sc + "px", width: b.W * sc + "px", height: b.H * sc + "px" });
      d.innerHTML = `<span class="ov-handle">◢</span>`;
      d.onpointerdown = e => startDrag(e, i, d, sc, e.target.classList.contains("ov-handle"));
      host.appendChild(d);
    });
  }
  window.addEventListener("resize", drawBoxes);
  $("#ov-prev").onload = drawBoxes;
  function select(i) {
    selected = i;
    document.querySelectorAll("#ov-layers .layer").forEach((el, j) => el.classList.toggle("sel", j === i));
    document.querySelectorAll("#ov-boxes .ov-box").forEach(el => el.classList.remove("sel"));
    const card = document.querySelectorAll("#ov-layers .layer")[i];
    if (card) card.scrollIntoView({ block: "nearest", behavior: "smooth" });
  }
  function startDrag(e, i, d, sc, resize) {
    e.preventDefault(); select(i); d.classList.add("sel"); d.setPointerCapture(e.pointerId);
    const o = cam.overlays[i], b = last.boxes[i];
    const x0 = e.clientX, y0 = e.clientY, l0 = b.X * sc, t0 = b.Y * sc, w0 = b.W * sc, h0 = b.H * sc;
    const move = ev => {
      const dx = ev.clientX - x0, dy = ev.clientY - y0;
      if (resize) { const k = Math.max(0.2, (w0 + dx) / w0); d.style.width = w0 * k + "px"; d.style.height = h0 * k + "px"; }
      else { d.style.left = l0 + (o.full_width ? 0 : dx) + "px"; d.style.top = t0 + dy + "px"; }
    };
    const up = () => {
      d.removeEventListener("pointermove", move); d.removeEventListener("pointerup", up);
      const W = last.width, H = last.height;
      const k = parseFloat(d.style.width) / w0;
      if (resize && Math.abs(k - 1) > 0.01) o.size = Math.round(o.size * k * 10) / 10;
      const w = parseFloat(d.style.width) / sc, h = parseFloat(d.style.height) / sc;
      const x = parseFloat(d.style.left) / sc, y = parseFloat(d.style.top) / sc;
      if (!resize || Math.abs(k - 1) > 0.01) placeAt(o, x, y, w, h, W, H);
      drawLayers(); select(i); preview();
    };
    d.addEventListener("pointermove", move); d.addEventListener("pointerup", up);
  }
  // converte una posizione in pixel in ancoraggio + margini percentuali
  // (scegliendo l'ancoraggio più vicino, così la posizione regge ai cambi di risoluzione)
  function placeAt(o, x, y, w, h, W, H) {
    x = Math.max(0, Math.min(W - w, x)); y = Math.max(0, Math.min(H - h, y)); // sempre dentro l'immagine
    const cx = (x + w / 2) / W, cy = (y + h / 2) / H;
    const curX = { left: 0, center: 0.5, right: 1 }[o.anchor === "center" ? "center" : o.anchor.split("-")[1]];
    const ax = o.full_width ? curX : cx < 1 / 3 ? 0 : cx > 2 / 3 ? 1 : 0.5;
    const ay = cy < 1 / 3 ? 0 : cy > 2 / 3 ? 1 : 0.5;
    const name = { 0: { 0: "top-left", 0.5: "top-center", 1: "top-right" }, 0.5: { 0: "middle-left", 0.5: "center", 1: "middle-right" }, 1: { 0: "bottom-left", 0.5: "bottom-center", 1: "bottom-right" } };
    o.anchor = name[ay][ax];
    const mx = ax === 0 ? x : ax === 1 ? W - w - x : x - (W - w) / 2;
    const my = ay === 0 ? y : ay === 1 ? H - h - y : y - (H - h) / 2;
    if (!o.full_width) o.offset_x = Math.round(mx / W * 1000) / 10;
    o.offset_y = Math.round(my / H * 1000) / 10;
  }

  const base = { enabled: true, offset_x: 2, offset_y: 3, color: "#ffffff", bold: false, shadow: true, background: "", bg_opacity: 0.5, opacity: 1, full_width: false };
  const addLayer = o => { cam.overlays.push({ ...base, ...o }); drawLayers(); preview(); };
  $("#ov-add-text").onclick = () => addLayer({ type: "text", text: "Scritta", anchor: "top-left", size: 4, bold: true });
  $("#ov-add-time").onclick = () => addLayer({ type: "text", text: "{date} {time}", anchor: "top-right", size: 3.5, background: "#000000", bg_opacity: 0.4 });
  $("#ov-add-data").onclick = () => {
    if (!cfg.data_sources.length) { toast("Configura prima una fonte in Dati live", true); return; }
    const d = cfg.data_sources[0];
    addLayer({ type: "text", text: `${d.name || "Temperatura"}: {data:${d.id}}`, anchor: "bottom-right", size: 3.5, background: "#000000", bg_opacity: 0.4 });
  };
  $("#ov-add-logo").onclick = () => addLayer({ type: "image", image: logos[0] || "", anchor: "top-right", size: 15, shadow: false });
  $("#ov-add-fc").onclick = () => {
    const om = cfg.data_sources.find(d => d.type === "open_meteo");
    addLayer({ type: "forecast", anchor: "top-right", size: 30, days: 3, start_tomorrow: false, show_rain: true, bold: true, background: "#000000", bg_opacity: 0.45,
      latitude: om ? om.latitude : (cfg.location.latitude || 44.2), longitude: om ? om.longitude : (cfg.location.longitude || 10.58) });
  };

  function drawLayers() {
    const box = $("#ov-layers");
    box.innerHTML = cam.overlays.length ? "" : `<div class="card help">Nessuna sovrimpressione. Aggiungine una con i pulsanti qui sopra.</div>`;
    cam.overlays.forEach((o, i) => {
      const el = document.createElement("div");
      el.className = "layer card" + (o.enabled ? "" : " off") + (i === selected ? " sel" : "");
      el.addEventListener("pointerdown", () => { if (selected !== i) { selected = i; document.querySelectorAll("#ov-layers .layer").forEach((x, j) => x.classList.toggle("sel", j === i)); drawBoxes(); } });
      const title = o.type === "image" ? "🖼️ " + (o.image || "logo") : o.type === "forecast" ? `⛅ Previsioni ${o.days || 3} giorni` : "🔤 " + (o.text || "").split("\n")[0];
      el.innerHTML = `<div class="layer-head"><input type="checkbox" ${o.enabled ? "checked" : ""} title="Attiva"><strong>${esc(title)}</strong>
        <button class="small" data-up title="Porta sotto">▲</button><button class="small" data-down title="Porta sopra">▼</button><button class="small" data-dup title="Duplica">⧉</button><button class="small danger" data-del title="Elimina">✕</button></div>
        <div style="display:flex;gap:14px;align-items:flex-start"><div><div class="help" style="margin-bottom:3px">Posizione</div><div class="anchor-grid">${anchorsList.map(([a, s]) => `<button data-anchor="${a}" class="${o.anchor === a ? "sel" : ""}">${s}</button>`).join("")}</div></div><div style="flex:1" data-form></div></div>`;
      const fields = o.type === "forecast" ? [
        { k: "coords", label: "Località delle previsioni", type: "coords" },
        { k: "days", label: "Giorni", type: "select", num: true, options: [[1, "1"], [2, "2"], [3, "3"], [4, "4"], [5, "5"]] },
        { k: "start_tomorrow", label: "Parti da domani", type: "checkbox" },
        { k: "show_rain", label: "Probabilità di pioggia", type: "checkbox" },
        { k: "size", label: "Larghezza (% immagine)", type: "range", min: 10, max: 80, step: 0.5 },
        { k: "color", label: "Colore testo", type: "color" },
        { k: "bold", label: "Grassetto", type: "checkbox" },
        { k: "shadow", label: "Ombra", type: "checkbox" },
        { k: "opacity", label: "Opacità", type: "range", min: 0.05, max: 1, step: 0.05 },
        { k: "offset_x", label: "Margine orizzontale %", type: "number", step: 0.5 },
        { k: "offset_y", label: "Margine verticale %", type: "number", step: 0.5 },
      ] : o.type === "image" ? [
        { k: "image", label: "Logo", type: "select", options: () => [["", "— scegli —"], ...logos.map(l => [l, l])] },
        { k: "size", label: "Larghezza (% immagine)", type: "range", min: 2, max: 60, step: 0.5 },
        { k: "opacity", label: "Opacità", type: "range", min: 0.05, max: 1, step: 0.05 },
        { k: "offset_x", label: "Margine orizzontale %", type: "number", step: 0.5 },
        { k: "offset_y", label: "Margine verticale %", type: "number", step: 0.5 },
      ] : [
        { k: "text", label: "Testo (più righe consentite)", type: "textarea", wide: true },
        { k: "size", label: "Dimensione carattere (% altezza)", type: "range", min: 1, max: 15, step: 0.1 },
        { k: "color", label: "Colore", type: "color" },
        { k: "bold", label: "Grassetto", type: "checkbox" },
        { k: "shadow", label: "Ombra", type: "checkbox" },
        { k: "opacity", label: "Opacità testo", type: "range", min: 0.05, max: 1, step: 0.05 },
        { k: "offset_x", label: "Margine orizzontale %", type: "number", step: 0.5 },
        { k: "offset_y", label: "Margine verticale %", type: "number", step: 0.5 },
      ];
      const bgState = { has_bg: !!o.background, bg_color: o.background || "#000000" };
      const form = renderForm(o, fields, preview);
      el.querySelector("[data-form]").appendChild(form);
      const bg = renderForm(bgState, [
        { k: "has_bg", label: "Riquadro di sfondo", type: "checkbox", redraw: true },
        { k: "bg_color", label: "Colore sfondo", type: "color", show: s => s.has_bg },
      ], () => { o.background = bgState.has_bg ? bgState.bg_color : ""; preview(); });
      const bg2 = renderForm(o, [
        { k: "bg_opacity", label: "Opacità sfondo", type: "range", min: 0, max: 1, step: 0.05 },
        ...(o.type === "text" ? [{ k: "full_width", label: "Fascia a tutta larghezza", type: "checkbox" }] : []),
      ], preview);
      el.querySelector("[data-form]").append(bg, bg2);
      const ta = el.querySelector("textarea");
      if (ta) { ta.addEventListener("focus", () => focused = ta); if (!focused) focused = ta; }
      el.querySelector(".layer-head input").onchange = e => { o.enabled = e.target.checked; el.classList.toggle("off", !o.enabled); preview(); };
      el.querySelectorAll("[data-anchor]").forEach(b => b.onclick = () => { o.anchor = b.dataset.anchor; drawLayers(); preview(); });
      el.querySelector("[data-del]").onclick = () => { cam.overlays.splice(i, 1); focused = null; drawLayers(); preview(); };
      el.querySelector("[data-dup]").onclick = () => { cam.overlays.splice(i + 1, 0, clone(o)); drawLayers(); preview(); };
      el.querySelector("[data-up]").onclick = () => { if (i > 0) { [cam.overlays[i - 1], cam.overlays[i]] = [cam.overlays[i], cam.overlays[i - 1]]; drawLayers(); preview(); } };
      el.querySelector("[data-down]").onclick = () => { if (i < cam.overlays.length - 1) { [cam.overlays[i + 1], cam.overlays[i]] = [cam.overlays[i], cam.overlays[i + 1]]; drawLayers(); preview(); } };
      box.appendChild(el);
    });
  }
  function drawLogos() {
    $("#ov-logos").innerHTML = logos.length ? logos.map(l => `<div class="logo-item"><img src="/api/logos/${encodeURIComponent(l)}" alt=""><span>${esc(l)}</span><button class="small danger" data-dl="${esc(l)}">elimina</button></div>`).join("") : `<span class="help">Nessun logo caricato.</span>`;
    $("#ov-logos").querySelectorAll("[data-dl]").forEach(b => b.onclick = async () => {
      if (!confirm("Eliminare il logo " + b.dataset.dl + "?")) return;
      await api("DELETE", "/api/logos/" + encodeURIComponent(b.dataset.dl));
      logos = await api("GET", "/api/logos"); drawLogos(); drawLayers(); preview();
    });
  }
  $("#ov-upload").onclick = e => busy(e.target, async () => {
    const f = $("#ov-file").files[0]; if (!f) { toast("Scegli un file", true); return; }
    const fd = new FormData(); fd.append("file", f);
    const r = await fetch("/api/logos", { method: "POST", body: fd });
    const j = await r.json(); if (!r.ok) throw new Error(j.error);
    logos = await api("GET", "/api/logos"); drawLogos();
    addLayer({ type: "image", image: j.name, anchor: "top-right", size: 15, shadow: false });
  });
  $("#ov-copy").onclick = () => {
    const others = cfg.cameras.filter(c => c.id !== cam.id);
    if (!others.length) { toast("Non ci sono altre webcam"); return; }
    openModal(`<h2 style="margin-top:0">Copia le sovrimpressioni da…</h2>${others.map(c => `<div class="list-item"><div class="grow">${esc(c.name || c.id)} <span class="help">${c.overlays.length} livelli</span></div><button data-from="${esc(c.id)}">Copia</button></div>`).join("")}`);
    $("#modal-body").querySelectorAll("[data-from]").forEach(b => b.onclick = () => {
      cam.overlays = clone(cfg.cameras.find(c => c.id === b.dataset.from).overlays);
      closeModal(); drawLayers(); preview();
    });
  };
  $("#ov-save").onclick = e => busy(e.target, async () => {
    const next = clone(cfg);
    next.cameras[next.cameras.findIndex(c => c.id === cam.id)].overlays = cam.overlays;
    await saveConfig(next);
  });
  drawLayers(); drawLogos(); preview();
};

// ======================================================================
// DATI LIVE
// ======================================================================
const meteoVars = [["temperature_2m", "Temperatura", "°C"], ["apparent_temperature", "Temperatura percepita", "°C"], ["relative_humidity_2m", "Umidità", "%"],
  ["wind_speed_10m", "Vento", " km/h"], ["wind_gusts_10m", "Raffiche", " km/h"], ["wind_direction_10m", "Direzione vento", "°"],
  ["pressure_msl", "Pressione", " hPa"], ["precipitation", "Precipitazioni", " mm"], ["cloud_cover", "Nuvolosità", "%"], ["uv_index", "Indice UV", ""]];
const wuVars = [["temp", "Temperatura", "°C"], ["humidity", "Umidità", "%"], ["windSpeed", "Vento", " km/h"], ["windGust", "Raffiche", " km/h"], ["winddir", "Direzione vento", "°"],
  ["pressure", "Pressione", " hPa"], ["precipRate", "Intensità pioggia", " mm/h"], ["precipTotal", "Pioggia oggi", " mm"], ["dewpt", "Punto di rugiada", "°C"],
  ["heatIndex", "Indice di calore", "°C"], ["windChill", "Temperatura percepita (vento)", "°C"], ["uv", "Indice UV", ""], ["solarRadiation", "Radiazione solare", " W/m²"]];
const dataFields = [
  { k: "name", label: "Nome", placeholder: "Es. Temperatura esterna" },
  { k: "id", label: "ID (per il segnaposto {data:ID})" },
  { k: "type", label: "Fonte", type: "select", redraw: true, options: [["open_meteo", "Meteo Open-Meteo (gratis, senza chiave)"], ["wunderground", "Stazione Weather Underground"], ["home_assistant", "Sensore di Home Assistant"], ["http_json", "URL JSON"], ["http_text", "URL testo semplice"]] },
  { k: "coords", label: "Coordinate del punto", type: "coords", show: d => d.type === "open_meteo" },
  { k: "variable", label: "Grandezza", type: "select", show: d => d.type === "open_meteo", options: meteoVars.map(v => [v[0], v[1]]) },
  { k: "station", label: "ID stazione", show: d => d.type === "wunderground", placeholder: "es. ISANTA123", help: "Usa “Cerca stazioni Weather Underground” nella pagina Dati live per trovarla. Se la chiave qui sotto è vuota si usa quella generale." },
  { k: "variable", label: "Grandezza", type: "select", show: d => d.type === "wunderground", options: wuVars.map(v => [v[0], v[1]]) },
  { k: "url", label: "Indirizzo Home Assistant", show: d => d.type === "home_assistant", placeholder: "http://homeassistant.local:8123", wide: true },
  { k: "url", label: "URL", show: d => d.type === "http_json" || d.type === "http_text", wide: true },
  { k: "token", label: "Chiave / token", type: "password", show: d => d.type !== "open_meteo", help: "Weather Underground: chiave API (gratuita per chi ha una stazione: wunderground.com → My Profile → API Keys). Home Assistant: token di accesso a lunga durata. Altri: bearer token opzionale." },
  { k: "entity", label: "Entità", show: d => d.type === "home_assistant", placeholder: "sensor.temperatura_esterna" },
  { k: "path", label: "Campo da leggere", show: d => d.type === "http_json" || d.type === "home_assistant", placeholder: "es. main.temp oppure attributes.humidity", help: "Percorso puntato nel JSON. Per HA vuoto = stato del sensore." },
  { k: "decimals", label: "Decimali", type: "number", min: -1, max: 4, help: "-1 = come arriva" },
  { k: "unit", label: "Unità (suffisso)", placeholder: "°C" },
  { k: "refresh_seconds", label: "Aggiorna ogni (secondi)", type: "number", min: 30 },
];
pages.dati = async (page, arg) => {
  if (arg) return editData(page, arg);
  page.innerHTML = `<h1>Dati live</h1><p class="help">Valori aggiornati automaticamente (temperatura, vento, sensori…) da usare nelle sovrimpressioni con <code>{data:ID}</code>.</p>
    <div class="toolbar"><a class="btn primary" href="#dati/nuovo">➕ Nuovo dato</a><button id="wu-open">🔎 Cerca stazioni Weather Underground</button></div><div class="card" id="dt-list"></div>
    <div class="card hidden" id="wu-box"></div>`;
  $("#wu-open").onclick = () => { $("#wu-box").classList.toggle("hidden"); wuSearchUI($("#wu-box")); };
  const st = await api("GET", "/api/status");
  $("#dt-list").innerHTML = cfg.data_sources.length ? cfg.data_sources.map(d => {
    const v = st.data[d.id] || {};
    return `<div class="list-item"><div class="grow"><b>${esc(d.name || d.id)}</b> <code>{data:${esc(d.id)}}</code><div class="help">${esc({ open_meteo: "Open-Meteo", wunderground: "Weather Underground " + (d.station || ""), home_assistant: "Home Assistant", http_json: "URL JSON", http_text: "URL testo" }[d.type] || d.type)} · ogni ${d.refresh_seconds}s ${v.error ? `· <span class="err-text">${esc(v.error)}</span>` : ""}</div></div>
      <b style="font-size:18px">${esc(v.value || "—")}</b><a class="btn small" href="#dati/${encodeURIComponent(d.id)}">Modifica</a></div>`;
  }).join("") : `<span class="help">Nessun dato configurato.</span>`;
};
// Ricerca delle stazioni Weather Underground vicine e scelta dei valori da
// mettere in sovrimpressione.
function wuSearchUI(box) {
  const q = { key: cfg.wu_api_key || "", query: cfg.location.name ? cfg.location.name.replace(/\(.*?\)/g, "").replace(/camping\s+\S+/i, "").trim() : "", latitude: cfg.location.latitude || 0, longitude: cfg.location.longitude || 0, mode: cfg.location.latitude ? "coords" : "name" };
  box.innerHTML = `<h3 style="margin-top:0">Stazioni meteo Weather Underground in zona</h3>
    <p class="help">Serve una chiave API (gratuita per chi possiede una stazione: wunderground.com → My Profile → API Keys).</p><div id="wu-form"></div>
    <div class="toolbar" style="margin:12px 0 0"><button class="primary" id="wu-go">🔎 Cerca stazioni vicine</button></div><div id="wu-res"></div>`;
  $("#wu-form").appendChild(renderForm(q, [
    { k: "key", label: "Chiave API Weather Underground", type: "password", wide: true },
    { k: "mode", label: "Cerca vicino a", type: "select", redraw: true, options: [["name", "una località (nome)"], ["coords", "coordinate GPS"]] },
    { k: "query", label: "Località", show: x => x.mode === "name", placeholder: "Sant'Anna Pelago" },
    { k: "coords", label: "Punto", type: "coords", show: x => x.mode === "coords" },
  ]));
  $("#wu-go").onclick = e => busy(e.target, async () => {
    if (q.key && q.key !== cfg.wu_api_key) { const next = clone(cfg); next.wu_api_key = q.key; await saveConfig(next); q.key = cfg.wu_api_key; }
    const r = await api("POST", "/api/wu/search", { key: q.key, query: q.mode === "name" ? q.query : "", latitude: q.latitude, longitude: q.longitude });
    const res = $("#wu-res");
    res.innerHTML = `<h3>${r.stations.length} stazioni ${r.place ? "vicino a " + esc(r.place) : ""}</h3>` + (r.stations.length ? `<table><thead><tr><th>Stazione</th><th>ID</th><th>Distanza</th><th></th></tr></thead><tbody>${r.stations.map(st => `<tr>
      <td>${esc(st.name || "—")}</td><td><code>${esc(st.id)}</code></td><td>${st.distance_km.toFixed(1)} km</td>
      <td style="white-space:nowrap"><a class="btn small" target="_blank" rel="noopener" href="https://www.wunderground.com/dashboard/pws/${encodeURIComponent(st.id)}">Pagina ↗</a> <button class="small primary" data-st="${esc(st.id)}" data-name="${esc(st.name)}">Valori attuali</button></td></tr>`).join("")}</tbody></table>` : `<p class="help">Nessuna stazione trovata.</p>`);
    res.querySelectorAll("[data-st]").forEach(b => b.onclick = ev => busy(ev.target, () => wuValues(b.dataset.st, b.dataset.name, q.key)));
  });
}
async function wuValues(station, name, key) {
  const r = await api("POST", "/api/wu/observation", { key, station });
  const fmt = v => String(Math.round(v.value * 10) / 10).replace(".", ",") + v.unit;
  openModal(`<h2 style="margin-top:0">${esc(name || station)} <span class="help">${esc(station)}</span></h2>
    <p class="help">Rilevazione delle ${new Date(r.time).toLocaleString("it-IT")}. Spunta i valori da mostrare in sovrimpressione.</p>
    <table>${r.values.map((v, i) => `<tr><td><input type="checkbox" data-i="${i}" ${["temp", "humidity", "windSpeed"].includes(v.variable) ? "checked" : ""}></td><td>${esc(v.label)}</td><td><b>${esc(fmt(v))}</b></td></tr>`).join("")}</table>
    <div class="toolbar" style="margin-top:14px"><label class="help">Aggiungi la scritta alla webcam <select id="wu-cam" style="width:auto"><option value="">— solo crea i dati live —</option>${cfg.cameras.map(c => `<option value="${esc(c.id)}">${esc(c.name || c.id)}</option>`).join("")}</select></label>
    <span class="spacer"></span><button class="primary" id="wu-create">Crea dati live</button></div>`);
  $("#wu-create").onclick = e => busy(e.target, async () => {
    const chosen = [...document.querySelectorAll("#modal [data-i]:checked")].map(c => r.values[Number(c.dataset.i)]);
    if (!chosen.length) throw new Error("Seleziona almeno un valore");
    const next = clone(cfg), parts = [];
    for (const v of chosen) {
      const id = uniqueId(`wu-${station}-${v.variable}`, next.data_sources);
      next.data_sources.push({ id, name: v.label, type: "wunderground", station, variable: v.variable, token: "", unit: v.unit, decimals: ["humidity", "winddir", "uv"].includes(v.variable) ? 0 : 1, refresh_seconds: 300, latitude: 0, longitude: 0, url: "", entity: "", path: "" });
      parts.push(`${v.label} {data:${id}}`);
    }
    const camId = $("#wu-cam").value;
    if (camId) {
      const c = next.cameras.find(c => c.id === camId);
      c.overlays.push({ type: "text", enabled: true, text: parts.join("  ·  "), anchor: "top-left", offset_x: 1.5, offset_y: 2, size: 3.2, color: "#ffffff", bold: true, shadow: true, background: "#000000", bg_opacity: 0.4, opacity: 1, full_width: false });
    }
    await saveConfig(next); closeModal();
    toast(camId ? "Dati creati e aggiunti in sovrimpressione: puoi spostarli con il mouse" : "Dati live creati");
    location.hash = camId ? "#sovrimpressioni/" + encodeURIComponent(camId) : "#dati";
    if (!camId) route();
  });
}

async function editData(page, id) {
  const isNew = id === "nuovo";
  const ds = isNew ? { id: "", name: "Temperatura", type: "open_meteo", latitude: cfg.location.latitude || 44.2, longitude: cfg.location.longitude || 10.58, variable: "temperature_2m", decimals: 1, unit: "°C", refresh_seconds: 600, url: "", token: "", entity: "", path: "" }
    : clone(cfg.data_sources.find(d => d.id === id) || {});
  if (!ds.type) { page.innerHTML = `<div class="card">Non trovato.</div>`; return; }
  if (isNew) ds.id = uniqueId(ds.name, cfg.data_sources);
  const origId = ds.id;
  page.innerHTML = `<h1>${isNew ? "Nuovo dato live" : esc(ds.name || ds.id)}</h1><div class="card" id="dt-form"></div>
    <div class="toolbar"><button class="primary" id="dt-save">💾 Salva</button><button id="dt-test">Prova lettura</button><span id="dt-res"></span><span class="spacer"></span><a class="btn" href="#dati">Annulla</a>${isNew ? "" : '<button class="danger" id="dt-del">Elimina</button>'}</div>`;
  const form = renderForm(ds, dataFields, k => {
    if (k === "variable") { const v = (ds.type === "wunderground" ? wuVars : meteoVars).find(m => m[0] === ds.variable); if (v) { ds.unit = v[2]; ds.name = v[1]; form.redraw(); } }
    if (k === "type") { if (ds.type === "wunderground" && !wuVars.some(v => v[0] === ds.variable)) ds.variable = "temp"; if (ds.type === "open_meteo" && !meteoVars.some(v => v[0] === ds.variable)) ds.variable = "temperature_2m"; form.redraw(); }
    if (k === "name" && isNew) { ds.id = uniqueId(ds.name, cfg.data_sources); }
  });
  $("#dt-form").appendChild(form);
  $("#dt-test").onclick = e => busy(e.target, async () => {
    try { const r = await api("POST", "/api/test-datasource", ds); $("#dt-res").innerHTML = `<span class="ok-text">✔ ${esc(r.value)}</span>`; }
    catch (err) { $("#dt-res").innerHTML = `<span class="err-text">✖ ${esc(err.message)}</span>`; }
  });
  $("#dt-save").onclick = e => busy(e.target, async () => {
    const next = clone(cfg);
    if (isNew) next.data_sources.push(ds); else next.data_sources[next.data_sources.findIndex(d => d.id === origId)] = ds;
    await saveConfig(next); location.hash = "#dati";
  });
  const del = $("#dt-del");
  if (del) del.onclick = e => { if (confirm("Eliminare questo dato?")) busy(e.target, async () => {
    const next = clone(cfg); next.data_sources = next.data_sources.filter(d => d.id !== origId);
    await saveConfig(next); location.hash = "#dati";
  }); };
}

// ======================================================================
// SITI FTP
// ======================================================================
const siteFields = [
  { k: "name", label: "Nome", placeholder: "Es. www.miosito.it" },
  { k: "id", label: "ID" },
  { k: "enabled", label: "Attivo", type: "checkbox" },
  { k: "protocol", label: "Protocollo", type: "select", redraw: true, options: [["ftp", "FTP"], ["ftps", "FTPS (FTP sicuro, TLS esplicito)"], ["sftp", "SFTP (SSH)"], ["onedrive", "Microsoft OneDrive"], ["folder", "Cartella locale / di rete"]] },
  { k: "host", label: "Server", show: s => isNet(s), placeholder: "ftp.miosito.it" },
  { k: "port", label: "Porta", type: "number", show: s => isNet(s) },
  { k: "username", label: "Utente", show: s => isNet(s) },
  { k: "password", label: "Password", type: "password", show: s => isNet(s) },
  { k: "oauth_client_id", label: "ID applicazione Microsoft (client ID)", show: s => s.protocol === "onedrive", help: "Si ottiene una volta registrando un'app su portal.azure.com (vedi guida nel README)." },
  { k: "oauth_tenant", label: "Tipo di account", type: "select", show: s => s.protocol === "onedrive", options: [["consumers", "OneDrive personale"], ["organizations", "OneDrive aziendale / Microsoft 365"]] },
  { k: "remote_dir", label: "Cartella", wide: true, placeholder: "/public_html/webcam", help: "FTP/SFTP: cartella sul server. OneDrive: cartella nel tuo OneDrive, es. Webcam/Porto. Cartella locale: percorso del PC, es. C:\\inetpub\\wwwroot\\webcam o la cartella OneDrive sincronizzata." },
  { k: "public_url", label: "Indirizzo web della cartella (opzionale)", wide: true, placeholder: "https://www.miosito.it/webcam/", help: "Serve per scrivere l'indirizzo sull'immagine con {site_url} o {history_url}." },
  { k: "history", label: "Pubblica anche il mini-sito con lo storico navigabile", type: "checkbox", wide: true, redraw: true },
  { k: "history_dir", label: "Sottocartella del mini-sito", show: s => s.history, help: "Il mini-sito sarà raggiungibile all'indirizzo …/cartella/storico/" },
  { k: "history_title", label: "Titolo del mini-sito", show: s => s.history, placeholder: "Webcam di …" },
  { k: "history_index", label: "Nome della pagina del mini-sito", show: s => s.history, placeholder: "index.html" },
  { k: "host_key", label: "Impronta SSH memorizzata", show: s => s.protocol === "sftp", readonly: true, help: "Registrata al primo collegamento. Svuotala se il server è stato cambiato." },
];
function isNet(s) { return ["ftp", "ftps", "sftp"].includes(s.protocol); }
pages.siti = async (page, arg) => {
  if (arg) return editSite(page, arg);
  page.innerHTML = `<h1>Siti FTP</h1><p class="help">Le destinazioni dove pubblicare le immagini, ognuna con le sue webcam e la sua periodicità.</p>
    <div class="toolbar"><a class="btn primary" href="#siti/nuovo">➕ Nuovo sito</a></div><div class="card" id="si-list"></div>`;
  const st = await api("GET", "/api/status");
  $("#si-list").innerHTML = cfg.sites.length ? cfg.sites.map(s => {
    const errs = st.pubs.filter(p => p.site_id === s.id && p.last_error).length + (st.sites[s.id]?.history_error ? 1 : 0);
    const pend = st.sites[s.id]?.history_pending;
    return `<div class="list-item"><div class="grow"><b>${esc(s.name || s.id)}</b> ${s.enabled ? "" : '<span class="badge">disattivato</span>'} ${errs ? `<span class="badge err">${errs} errori</span>` : ""}
      <div class="help">${esc(s.protocol.toUpperCase())} ${esc(s.host || s.remote_dir)} · ${s.publications.length} pubblicazioni${s.history ? " · mini-sito storico" + (pend ? ` (${pend} immagini da inviare)` : "") : ""}</div></div>
      <a class="btn small" href="#siti/${encodeURIComponent(s.id)}">Modifica</a></div>`;
  }).join("") : `<span class="help">Nessun sito configurato.</span>`;
};
async function editSite(page, id) {
  const isNew = id === "nuovo";
  const site = isNew ? { id: "", name: "", enabled: true, protocol: "ftp", host: "", port: 21, username: "", password: "", remote_dir: "", public_url: "", oauth_client_id: "", oauth_tenant: "consumers", oauth_refresh: "", publications: [], history: true, history_dir: "storico", history_title: "", history_index: "index.html", host_key: "" }
    : clone(cfg.sites.find(s => s.id === id) || {});
  if (!site.protocol) { page.innerHTML = `<div class="card">Sito non trovato.</div>`; return; }
  const origId = site.id;
  page.innerHTML = `<h1>${isNew ? "Nuovo sito" : esc(site.name || site.id)}</h1>
    <div class="card" id="si-form"></div>
    <div class="card"><h3 style="margin-top:0">Webcam pubblicate su questo sito</h3>
      <table><thead><tr><th>Webcam</th><th>Nome file sul sito</th><th>Periodicità</th><th></th></tr></thead><tbody id="si-pubs"></tbody></table>
      <div class="toolbar" style="margin:10px 0 0"><button id="si-addpub">➕ Aggiungi webcam</button></div>
      <p class="help">Esempio di codice da mettere nella pagina del sito: <code>&lt;img src="webcam-porto.jpg" alt="Webcam"&gt;</code>. Le immagini vengono caricate con un nome temporaneo e poi rinominate: i visitatori non vedono mai un'immagine a metà.</p></div>
    <div class="toolbar"><button class="primary" id="si-save">💾 Salva</button><button id="si-test">🔌 Test pubblicazione</button><span id="si-res"></span><span class="spacer"></span>
      ${isNew ? "" : '<button id="si-resync" title="Ricarica tutto lo storico sul sito">Ripubblica storico</button><button class="danger" id="si-del">Elimina</button>'}<a class="btn" href="#siti">Annulla</a></div>`;
  const form = renderForm(site, siteFields, k => {
    if (k === "protocol") { site.port = { ftp: 21, ftps: 21, sftp: 22 }[site.protocol] || 0; if (site.protocol === "onedrive" && !site.oauth_tenant) site.oauth_tenant = "consumers"; form.redraw(); drawOd(); }
    if (k === "name" && isNew) { site.id = uniqueId(site.name, cfg.sites); form.redraw(); }
  });
  $("#si-form").appendChild(form);
  const od = document.createElement("div");
  $("#si-form").appendChild(od);
  const drawOd = () => {
    if (site.protocol !== "onedrive") { od.innerHTML = ""; return; }
    od.innerHTML = `<div class="toolbar" style="margin:14px 0 0">${site.oauth_refresh ? '<span class="badge ok">account collegato</span>' : '<span class="badge warn">account non collegato</span>'}
      <button id="od-connect">🔗 ${site.oauth_refresh ? "Ricollega" : "Collega"} account Microsoft</button><span class="help">Salva il sito prima di collegarlo.</span></div>`;
    $("#od-connect").onclick = e => busy(e.target, async () => {
      if (isNew || JSON.stringify(cfg.sites.find(s => s.id === origId)) !== JSON.stringify(site)) {
        const next = clone(cfg);
        if (isNew) next.sites.push(site); else next.sites[next.sites.findIndex(s => s.id === origId)] = site;
        await saveConfig(next);
      }
      const r = await api("POST", `/api/sites/${encodeURIComponent(site.id)}/onedrive-connect`);
      openModal(`<h2 style="margin-top:0">Collega OneDrive</h2><p>1. Apri <a href="${esc(r.verification_uri)}" target="_blank" rel="noopener">${esc(r.verification_uri)}</a></p>
        <p>2. Inserisci il codice: <b style="font-size:24px;letter-spacing:2px">${esc(r.user_code)}</b></p><p>3. Accedi con l'account Microsoft e accetta.</p><p class="help" id="od-st">In attesa…</p>`);
      const t = setInterval(async () => {
        const f = await api("GET", "/api/onedrive-flow/" + r.flow).catch(() => null);
        if (!f || f.status === "pending") return;
        clearInterval(t);
        if (f.status === "ok") { closeModal(); toast("Account Microsoft collegato"); await loadConfig(); route(); }
        else { const el = $("#od-st"); if (el) el.innerHTML = `<span class="err-text">${esc(f.error)}</span>`; }
      }, 3000);
    });
  };
  drawOd();
  const drawPubs = () => {
    const tb = $("#si-pubs");
    tb.innerHTML = site.publications.length ? "" : `<tr><td colspan="4" class="help">Nessuna webcam: aggiungine una.</td></tr>`;
    site.publications.forEach((p, i) => {
      const tr = document.createElement("tr");
      tr.innerHTML = `<td><select>${cfg.cameras.map(c => `<option value="${esc(c.id)}" ${c.id === p.camera_id ? "selected" : ""}>${esc(c.name || c.id)}</option>`).join("")}</select></td>
        <td><input value="${esc(p.filename)}"></td><td>${intervalSelect(p.interval_seconds)}</td><td><button class="small danger">✕</button></td>`;
      const [cs, fn, is] = tr.querySelectorAll("select, input");
      cs.onchange = () => p.camera_id = cs.value;
      fn.oninput = () => p.filename = fn.value;
      is.onchange = () => p.interval_seconds = Number(is.value);
      tr.querySelector("button").onclick = () => { site.publications.splice(i, 1); drawPubs(); };
      tb.appendChild(tr);
    });
  };
  $("#si-addpub").onclick = () => {
    if (!cfg.cameras.length) { toast("Aggiungi prima una webcam", true); return; }
    const c = cfg.cameras.find(c => !site.publications.some(p => p.camera_id === c.id)) || cfg.cameras[0];
    site.publications.push({ camera_id: c.id, filename: c.id + ".jpg", interval_seconds: 120 });
    drawPubs();
  };
  drawPubs();
  $("#si-test").onclick = e => busy(e.target, async () => {
    $("#si-res").textContent = "Prova in corso…";
    const r = await fetch("/api/test-site", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(site) });
    const j = await r.json();
    const steps = (j.steps || []).map(x => `<div class="ok-text">✔ ${esc(x)}</div>`).join("");
    $("#si-res").textContent = "";
    openModal(`<h2 style="margin-top:0">Test di pubblicazione: ${esc(site.name || site.id)}</h2>${steps}
      ${j.error ? `<div class="err-text">✖ ${esc(j.error)}</div><p class="help">Controlla server, porta, utente, password e cartella. Molti hosting usano come cartella /public_html o /httpdocs.</p>` : `<p><b class="ok-text">La pubblicazione su questo sito funziona.</b></p>`}`);
  });
  $("#si-save").onclick = e => busy(e.target, async () => {
    const next = clone(cfg);
    if (isNew) next.sites.push(site); else next.sites[next.sites.findIndex(s => s.id === origId)] = site;
    await saveConfig(next); location.hash = "#siti";
  });
  if (!isNew) {
    $("#si-resync").onclick = e => { if (confirm("Ricaricare sul sito tutte le immagini dello storico? Utile se hai cambiato server o cancellato la cartella.")) busy(e.target, async () => { await api("POST", `/api/sites/${encodeURIComponent(origId)}/resync-history`); toast("Lo storico verrà ricaricato gradualmente"); }); };
    $("#si-del").onclick = e => { if (confirm("Eliminare il sito? I file già pubblicati restano sul server.")) busy(e.target, async () => {
      const next = clone(cfg); next.sites = next.sites.filter(s => s.id !== origId);
      await saveConfig(next); location.hash = "#siti";
    }); };
  }
}

// ======================================================================
// STORICO
// ======================================================================
pages.storico = async (page) => {
  page.innerHTML = `<h1>Storico</h1><p class="help">Lo stesso visualizzatore viene pubblicato come mini-sito sui siti con l'opzione “storico” attiva.</p><iframe class="history" src="/history/"></iframe>`;
};

// ======================================================================
// IMPOSTAZIONI
// ======================================================================
pages.impostazioni = async (page, arg) => {
  const tab = arg || "generali";
  const tabs = [["generali", "Generali"], ["notifiche", "Notifiche email"], ["aggiornamenti", "Aggiornamenti"], ["backup", "Backup e replica"], ["diagnostica", "Diagnostica"]];
  page.innerHTML = `<h1>Impostazioni</h1><div class="tabs">${tabs.map(([k, l]) => `<button class="${k === tab ? "sel" : ""}" onclick="location.hash='#impostazioni/${k}'">${l}</button>`).join("")}</div><div id="set-body"></div>`;
  const body = $("#set-body");
  const info = await api("GET", "/api/info");

  if (tab === "generali") {
    const s = { admin_user: cfg.admin_user, admin_password: cfg.admin_password, listen: cfg.listen };
    const loc = clone(cfg.location);
    body.innerHTML = `<div class="card"><h3 style="margin-top:0">Località</h3><div id="g-loc"></div></div><div class="card"><h3 style="margin-top:0">Accesso al pannello</h3><div id="g-form"></div></div><div class="toolbar"><button class="primary" id="g-save">💾 Salva</button></div>
      <div class="card"><h3 style="margin-top:0">Informazioni</h3><table>
      <tr><td>Versione</td><td><b>${esc(info.version)}</b></td></tr><tr><td>Computer</td><td>${esc(info.hostname)}</td></tr>
      <tr><td>Cartella dati</td><td><code>${esc(info.data_dir)}</code></td></tr>
      <tr><td>ffmpeg (per RTSP)</td><td id="ff-cell"></td></tr></table></div>`;
    const ffDraw = async () => {
      const f = await api("GET", "/api/ffmpeg"); const cell = $("#ff-cell"); if (!cell) return;
      cell.innerHTML = f.found ? `<span class="badge ok">trovato</span> <span class="help">${esc(f.path)}</span>`
        : f.running ? '<span class="badge warn">download in corso…</span>'
        : `<span class="badge warn">non trovato</span> <button class="small" id="ff-inst">⬇️ Installa ffmpeg</button> ${f.error ? `<span class="err-text">${esc(f.error)}</span>` : ""}`;
      const b = $("#ff-inst"); if (b) b.onclick = e => busy(e.target, async () => { await api("POST", "/api/ffmpeg/install"); ffDraw(); });
      if (f.running) setTimeout(ffDraw, 3000);
    };
    ffDraw();
    $("#g-loc").appendChild(renderForm(loc, [
      { k: "name", label: "Nome della località", wide: true, placeholder: "Camping Coggiolo Sant'Anna Pelago (MO)", help: "Segnaposto {location} nelle scritte." },
      { k: "altitude", label: "Altitudine (m s.l.m.)", type: "number", help: "Segnaposto {altitude}." },
      { k: "coords", label: "Coordinate GPS", type: "coords", help: "Usate come punto predefinito per meteo, previsioni e ricerca stazioni. In Google Maps: clic destro sul punto → clic sulle coordinate per copiarle, poi incollale qui." },
    ]));
    $("#g-form").appendChild(renderForm(s, [
      { k: "admin_user", label: "Utente del pannello" },
      { k: "admin_password", label: "Password del pannello", type: "password" },
      { k: "listen", label: "Porta del pannello", help: "Es. :8080 — la modifica ha effetto al riavvio del servizio." },
    ]));
    $("#g-save").onclick = e => busy(e.target, async () => {
      const next = clone(cfg); Object.assign(next, s); next.location = loc;
      await saveConfig(next);
      if (s.admin_password !== "********") toast("Password cambiata: il browser la richiederà");
    });
  }

  if (tab === "notifiche") {
    const al = clone(cfg.alerts);
    body.innerHTML = `<div class="card"><p class="help" style="margin-top:0">Ricevi un'email quando una webcam non risponde o una pubblicazione non riesce. L'email indica il nome del computer (${esc(info.hostname)}).</p><div id="al-form"></div></div>
      <div class="toolbar"><button class="primary" id="al-save">💾 Salva</button><button id="al-test">✉️ Invia email di prova</button><span id="al-res"></span></div>
      <div class="card help">Gmail: server <code>smtp.gmail.com</code>, porta 587, STARTTLS, come password una “password per le app” (Account Google → Sicurezza). Outlook/Hotmail: <code>smtp-mail.outlook.com</code>, porta 587, STARTTLS.</div>`;
    $("#al-form").appendChild(renderForm(al, [
      { k: "enabled", label: "Notifiche attive", type: "checkbox", wide: true },
      { k: "to", label: "Destinatari (separati da virgola)", wide: true, placeholder: "io@esempio.it, tecnico@esempio.it" },
      { k: "smtp_host", label: "Server SMTP", placeholder: "smtp.gmail.com" },
      { k: "smtp_port", label: "Porta", type: "number" },
      { k: "security", label: "Sicurezza", type: "select", options: [["starttls", "STARTTLS (porta 587)"], ["tls", "SSL/TLS (porta 465)"], ["none", "Nessuna (sconsigliato)"]] },
      { k: "username", label: "Utente SMTP" },
      { k: "password", label: "Password SMTP", type: "password" },
      { k: "from", label: "Mittente", placeholder: "vuoto = utente SMTP" },
      { k: "after_minutes", label: "Avvisa se l'errore dura da (minuti)", type: "number", min: 1 },
      { k: "repeat_hours", label: "Ripeti l'avviso ogni (ore, 0 = mai)", type: "number", min: 0 },
      { k: "notify_recovery", label: "Avvisa anche quando il problema si risolve", type: "checkbox" },
    ], k => { if (k === "security") { al.smtp_port = al.security === "tls" ? 465 : 587; } }));
    $("#al-save").onclick = e => busy(e.target, async () => { const next = clone(cfg); next.alerts = al; await saveConfig(next); });
    $("#al-test").onclick = e => busy(e.target, async () => {
      try { const r = await api("POST", "/api/test-email", al); $("#al-res").innerHTML = `<span class="ok-text">✔ ${esc(r.ok)}</span>`; }
      catch (err) { $("#al-res").innerHTML = `<span class="err-text">✖ ${esc(err.message)}</span>`; }
    });
  }

  if (tab === "aggiornamenti") {
    const u = clone(cfg.update);
    body.innerHTML = `<div class="card"><h3 style="margin-top:0">Versione installata: ${esc(info.version)}</h3><div id="up-status" class="help">Caricamento…</div>
      <div class="toolbar" style="margin:12px 0 0"><button id="up-check">🔄 Controlla ora</button><button class="primary hidden" id="up-apply">⬇️ Installa aggiornamento</button></div></div>
      <div class="card"><h3 style="margin-top:0">Fonte degli aggiornamenti</h3><div id="up-form"></div>
      <p class="help">Con <b>GitHub</b> ogni nuova release del progetto viene vista da tutte le installazioni. Con <b>URL</b> carichi tu <code>latest.json</code> e gli eseguibili su un tuo sito.</p>
      <div class="toolbar" style="margin:0"><button class="primary" id="up-save">💾 Salva</button></div></div>
      <div class="card"><h3 style="margin-top:0">Aggiornamento manuale</h3><p class="help">Carica direttamente un nuovo eseguibile (es. una versione di prova ricevuta per email). Il programma si riavvia da solo.</p>
      <div class="toolbar" style="margin:0"><input type="file" id="up-file" style="width:auto"><button id="up-upload">Carica e installa</button></div></div>`;
    $("#up-form").appendChild(renderForm(u, [
      { k: "source", label: "Fonte", type: "select", redraw: true, options: [["", "Aggiornamenti disattivati"], ["github", "GitHub Releases"], ["url", "URL di latest.json"]] },
      { k: "repo", label: "Repository GitHub", placeholder: "lucapaoli74/hassioluca", show: x => x.source === "github" },
      { k: "token", label: "Token GitHub (solo repository privati)", type: "password", show: x => x.source === "github", help: "Token “fine-grained” con permesso Contents: read-only sul repository." },
      { k: "url", label: "URL di latest.json", wide: true, show: x => x.source === "url", placeholder: "https://www.miosito.it/webcam-manager/latest.json" },
      { k: "auto", label: "Installa automaticamente le nuove versioni", type: "checkbox", show: x => !!x.source },
      { k: "check_hours", label: "Controlla ogni (ore)", type: "number", min: 1, show: x => !!x.source },
    ]));
    const draw = st => {
      $("#up-status").innerHTML = st.error ? `<span class="err-text">${esc(st.error)}</span>`
        : st.latest ? (st.available ? `<b class="ok-text">Disponibile la versione ${esc(st.latest)}</b><pre class="log" style="max-height:200px">${esc(st.notes || "")}</pre>` : `Sei aggiornato (ultima versione ${esc(st.latest)}).`)
          : "Non ancora controllato.";
      if (st.checked_at && !st.checked_at.startsWith("0001")) $("#up-status").insertAdjacentHTML("beforeend", `<div class="help">Ultimo controllo ${ago(st.checked_at)} · piattaforma ${esc(st.platform)}</div>`);
      $("#up-apply").classList.toggle("hidden", !st.available);
    };
    draw(await api("GET", "/api/update"));
    $("#up-check").onclick = e => busy(e.target, async () => draw(await api("POST", "/api/update/check")));
    $("#up-apply").onclick = e => busy(e.target, async () => { const r = await api("POST", "/api/update/apply"); toast(r.ok); waitRestart(); });
    $("#up-save").onclick = e => busy(e.target, async () => { const next = clone(cfg); next.update = u; await saveConfig(next); });
    $("#up-upload").onclick = e => busy(e.target, async () => {
      const f = $("#up-file").files[0]; if (!f) { toast("Scegli il file", true); return; }
      if (!confirm(`Installare ${f.name}? Il programma verrà riavviato.`)) return;
      const r = await api("POST", "/api/update/upload", f); toast(r.ok); waitRestart();
    });
  }

  if (tab === "backup") {
    const list = await api("GET", "/api/backups");
    body.innerHTML = `<div class="card"><h3 style="margin-top:0">Replica su un altro computer</h3>
      <p class="help">Esporta la configurazione completa (webcam, sovrimpressioni, loghi, siti, dati live) e importala sull'altro PC. Porta e password del pannello di destinazione non vengono toccate.</p>
      <div class="toolbar" style="margin:0"><a class="btn primary" href="/api/export">⬇️ Esporta configurazione</a><input type="file" id="bk-file" accept=".zip" style="width:auto"><button id="bk-import">⬆️ Importa</button></div></div>
      <div class="card"><h3 style="margin-top:0">Copie di sicurezza automatiche</h3><p class="help">Una copia viene creata prima di ogni modifica (ultime 50). Ripristinane una per annullare modifiche fatte per errore.</p>
      ${list.length ? `<table>${list.map(n => `<tr><td>${esc(n.replace(/^config-(\d{4})(\d\d)(\d\d)-(\d\d)(\d\d)(\d\d).*$/, "$3/$2/$1 $4:$5:$6"))}</td><td style="text-align:right"><button class="small" data-r="${esc(n)}">Ripristina</button></td></tr>`).join("")}</table>` : '<span class="help">Nessuna copia ancora.</span>'}</div>`;
    $("#bk-import").onclick = e => busy(e.target, async () => {
      const f = $("#bk-file").files[0]; if (!f) { toast("Scegli il file .zip", true); return; }
      if (!confirm("La configurazione attuale verrà sostituita (ne resta una copia di sicurezza). Continuare?")) return;
      const r = await api("POST", "/api/import", f); toast(r.ok); cfg = null; await loadConfig();
    });
    body.querySelectorAll("[data-r]").forEach(b => b.onclick = e => { if (confirm("Ripristinare questa configurazione?")) busy(e.target, async () => { const r = await api("POST", `/api/backups/${b.dataset.r}/restore`); toast(r.ok); await loadConfig(); }); });
  }

  if (tab === "diagnostica") {
    body.innerHTML = `<div class="card"><h3 style="margin-top:0">Segnalare un problema</h3>
      <p class="help">Scarica il pacchetto di diagnostica (versione, configurazione <b>senza password</b>, stato e log) e allegalo alla segnalazione.</p>
      <div class="toolbar" style="margin:0"><a class="btn primary" href="/api/diagnostics">⬇️ Scarica diagnostica</a></div></div>
      <div class="card"><h3 style="margin-top:0">Log recente</h3><pre class="log" id="dg-log"></pre></div>`;
    every(5000, async () => {
      const lines = await api("GET", "/api/logs");
      const el = $("#dg-log"); if (!el) return;
      const atEnd = el.scrollTop + el.clientHeight >= el.scrollHeight - 10;
      el.textContent = lines.join("\n") || "(vuoto)";
      if (atEnd) el.scrollTop = el.scrollHeight;
    });
  }
};
function waitRestart() {
  openModal(`<h2 style="margin-top:0">Riavvio in corso…</h2><p class="help">La pagina si ricaricherà da sola appena il programma è di nuovo attivo.</p>`);
  let n = 0;
  const t = setInterval(async () => {
    n++;
    try { const r = await fetch("/api/info"); if (r.ok && n > 2) { clearInterval(t); location.reload(); } } catch (_) { }
  }, 2000);
}

// ---------- avvio ----------
async function setVersion() {
  try {
    const info = await api("GET", "/api/info");
    if (!cfg) await loadConfig();
    $("#version").textContent = `${cfg.location.name ? cfg.location.name + " · " : ""}v${info.version} · ${info.hostname}`;
    document.title = cfg.location.name ? `Webcam Manager · ${cfg.location.name}` : "Webcam Manager";
  } catch (_) { }
}
(async () => { await setVersion(); route(); })();
