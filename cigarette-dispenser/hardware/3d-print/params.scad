// Shared parameters — edit this file only, every part picks these up via
// `include`. Meccanismo: tramoggia (bulk, >=100 sigarette sfuse) -> rullo
// scanalato (singolarizzatore) -> scivolo -> pulsante frontale.
//
// MISURA LE TUE SIGARETTE PRIMA DI STAMPARE. Questi sono valori tipici per
// un formato king-size con filtro (~84mm x ~7.9mm) — verificalo con un
// calibro sulle sigarette che userai davvero (JPS o altro) e correggi.

cig_l      = 84;    // lunghezza sigaretta (mm)
cig_d      = 7.9;   // diametro sigaretta (mm)
clear_ax   = 2;      // gioco assiale (lungo il rullo) per lato
clear_rad  = 1.5;    // gioco radiale nella scanalatura

// --- Rullo scanalato ---------------------------------------------------
flutes       = 8;    // numero di scanalature attorno al rullo
wall         = 3.5;  // parete minima tra due scanalature (robustezza stampa)
shaft_d      = 6;    // foro per accoppiatore albero motore

groove_d   = cig_d + 2*clear_rad;                 // diametro scanalatura
flute_pitch_arc = groove_d + wall;                // passo tra scanalature (arco)
r_pitch    = flute_pitch_arc * flutes / (2*PI);   // raggio al centro delle scanalature
// Il grezzo del rullo si ferma esattamente al raggio r_pitch: ogni
// scanalatura è un foro cilindrico centrato SU quel raggio, quindi viene
// tagliata a metà dalla superficie del grezzo, lasciando un canale a "U"
// aperto verso l'esterno (non un foro cieco tangente alla superficie —
// un tentativo precedente con roller_r = r_pitch + groove_d/2 produceva
// una mesh non manifold, tangente in un solo punto).
roller_r   = r_pitch;                             // raggio esterno rullo (creste)
core_r     = r_pitch - groove_d/2;                // raggio al fondo scanalatura
roller_len = cig_l + 2*clear_ax;

// --- Alloggiamento (housing a "C") --------------------------------------
// Deve coprire anche la sporgenza della sigaretta oltre le creste (la
// scanalatura è più larga della sigaretta per il gioco, quindi la
// sigaretta non sta a filo — protrude di circa clear_rad..2*clear_rad).
housing_clear   = 4;                   // gioco radiale rullo/alloggiamento
housing_ir      = roller_r + housing_clear; // raggio interno alloggiamento
housing_wall    = 3;
housing_or      = housing_ir + housing_wall;
end_plate_t     = 4;

// apertura di carico (sopra, sotto la tramoggia) e di scarico (sotto)
load_gap_deg    = 55;   // ampiezza apertura di carico
discharge_gap_deg = 40; // ampiezza apertura di scarico
// l'apertura di carico è centrata in alto (90°), quella di scarico in
// basso (270°) nel piano di rotazione del rullo (asse orizzontale, vista
// dal fronte lungo l'asse Y)

// --- Tramoggia (hopper) --------------------------------------------------
// Capienza richiesta: >=100 sigarette sfuse. Volume solido di 100 sigarette
// =~ 411 cm3; per cilindretti alla rinfusa il fattore di impaccamento reale
// è basso e variabile (si intasano/incrociano) — 35-50% è una stima
// prudente. Le dimensioni sotto danno un volume utile di ~1.3 L, che
// garantisce >=100 pezzi anche nello scenario pessimistico (35%): vedi il
// conto fatto per questo file (~111 sigarette al 35%, fino a ~158 al 50%).
hopper_top_w   = 130;
hopper_top_d   = 100;
hopper_bot_w   = roller_len + 6;   // combacia con la larghezza del rullo
hopper_bot_d   = 34;
hopper_h       = 190;
hopper_wall    = 2.4;

// --- Bordo/coperchio/serratura tramoggia (hopper.scad + hopper_lid.scad) -
rim_w        = 10;   // sporgenza del bordo oltre l'apertura
rim_t        = 4;
hinge_hole_d = 3.2;
hinge_x      = [-30, 30];  // posizione viti cerniera, lato -Y (fronte)
latch_hole_d = 4;          // foro per il gancio della serratura, lato +Y (retro)
reed_hole_d  = 6;          // foro per il sensore magnetico di sportello chiuso

// --- Mobile esterno (cabinet.scad) — pannelli piatti, non stampati -------
// Un guscio 3D pieno di queste dimensioni non è stampabile in un pezzo
// solo: qui si generano pannelli piatti (taglio laser/CNC/sega) che
// racchiudono il meccanismo, lasciando accessibili solo pulsante,
// feritoia e sportello tramoggia con serratura.
cab_w        = 180;   // larghezza (sx-dx)
cab_d        = 240;   // profondità (fronte-retro)
cab_h        = 420;   // altezza
panel_mat_t  = 9;     // spessore materiale pannelli (legno 9mm o simile — adatta)
cab_back_margin = 15; // margine tra il bordo posteriore e il ritaglio tramoggia
