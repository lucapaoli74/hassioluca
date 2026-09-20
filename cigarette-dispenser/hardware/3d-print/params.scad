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

// orecchiette di fissaggio sui piatti terminali (drive/idler): stesso
// calcolo usato in housing.scad, riportato qui come unica fonte di verità
// così il mobile (cabinet.scad) sa esattamente dove mettere i fori
// corrispondenti sui pannelli laterali, invece di indovinare le quote.
mount_ear_r      = housing_or - 6;   // raggio al centro dell'orecchietta
mount_hole_off   = 8;                // offset del foro dentro l'orecchietta
mount_hole_d     = 3.4;
mount_angles     = [45, 135, 225, 315];
mount_hole_r     = mount_ear_r + mount_hole_off;  // raggio dei fori dall'asse rullo

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

// --- Scivolo (chute.scad) -------------------------------------------------
drop_w  = roller_len + 6;   // combacia con l'apertura di scarico dell'housing
drop_d1 = discharge_gap_deg > 0 ? 2 * housing_or * sin(discharge_gap_deg/2) + 6 : 30;
drop_d2 = cig_d + 10;       // uscita: appena più larga della sigaretta
drop_h  = 60;

// --- Mobile esterno (cabinet.scad + front_panel.scad) — pannelli piatti --
// Un guscio 3D pieno di queste dimensioni non è stampabile in un pezzo
// solo: qui si generano pannelli piatti (stampabili o tagliabili) che
// racchiudono il meccanismo, lasciando accessibili solo pulsante,
// feritoia e sportello tramoggia con serratura.
//
// Non sono più quote a caso: il mobile è dimensionato dalla geometria
// reale del meccanismo (roller_len, housing_or, hopper_*, drop_h) così
// resta corretto se cambi le sigarette o il numero di scanalature in cima
// a questo file — e soprattutto i pannelli hanno fori nei punti giusti per
// avvitare l'housing e la tramoggia, non solo per contenerli a vista.
//
// Assi: X = larghezza (asse del rullo, orizzontale, centrato su 0), Y =
// profondità (0 = fronte del mobile), Z = altezza (0 = fondo del mobile).
// Il rullo/housing non stanno ruotati "a caso" per starci dentro: la
// rotazione che li porta in orizzontale con questa stessa convenzione di
// assi (rotate([90,0,90]) — verificata algebricamente, vedi assembly.scad)
// è definita una sola volta lì, e sia cabinet.scad sia front_panel.scad
// condividono queste coordinate senza doverla rifare.
panel_mat_t  = 9;     // spessore materiale pannelli (legno 9mm o simile — adatta)

side_wall_margin  = 12;  // gioco tra il cerchio dei fori housing e il bordo pannello
depth_margin      = 15;  // gioco tra la tramoggia e i pannelli fronte/retro
base_clear        = 35;  // spazio sotto lo scivolo per piedini/elettronica
mech_gap          = 12;  // gioco verticale tra tramoggia/scivolo e l'housing
lid_clear         = 15;  // spazio sopra la tramoggia per lo sportello

cab_w = max(hopper_top_w, roller_len + 2*end_plate_t) + 2*side_wall_margin;
cab_d = hopper_top_d + 2*depth_margin;

// centro del rullo/housing dentro al mobile (asse a X=0, cioè a metà cab_w)
mech_y = cab_d / 2;                        // profondità (centrato)
mech_z = base_clear + drop_h + housing_or; // altezza da terra

cab_h = lid_clear + hopper_h + mech_gap + 2*housing_or + drop_h + base_clear;

// il pannello frontale sta vicino al fronte (Y piccolo): lo scivolo deve
// quindi spostarsi in orizzontale dal centro housing (mech_y) fino a lì
front_y   = 14;                 // profondità del centro feritoia dal fronte
chute_dy  = mech_y - front_y;   // spostamento orizzontale richiesto allo scivolo

// il pannello frontale copre solo la parte bassa (rullo/scivolo): la
// tramoggia sopra resta "a vista", chiusa dalle sue stesse pareti piene —
// così il pannello frontale resta sotto i 250mm senza bisogno di spezzarlo
front_panel_h = mech_z + housing_or + mech_gap;

chute_top_z    = mech_z - housing_or - mech_gap;  // aggancio allo scarico housing
chute_bottom_z = chute_top_z - drop_h;             // uscita verso il pannello

cab_back_margin = depth_margin; // margine tra il bordo posteriore e il ritaglio tramoggia

// Piano di stampa Bambu X1C: 256x256x256mm. Qui sotto un margine di
// sicurezza (bordo letto, adesione, calibro) — nessun pezzo, pannelli del
// mobile compresi, deve superare questa misura in nessuna dimensione.
x1c_max      = 250;
seam_hole_d  = 4;      // fori M4 lungo la giunzione dei pannelli spezzati
seam_hole_n  = 5;

// motore NEMA17: fori esterni sul pannello laterale lato "drive" — il
// motore resta FUORI dal mobile (solo l'albero entra), niente bisogno di
// spazio interno dedicato al suo corpo
nema17_hole_spacing = 31;
nema17_hole_d        = 3.4;
nema17_shaft_bore_d  = 24;
idler_shaft_clear_d  = shaft_d + 2;  // foro di passaggio sul pannello lato folle

// segmento del pannello laterale/posteriore in cui cade il centro
// dell'housing, e sua quota locale dentro quel segmento — usati da
// cabinet.scad per mettere il cerchio di fissaggio housing nel pezzo
// giusto, senza tagliarlo a metà lungo una giunzione
cab_seg_n     = ceil(cab_h / x1c_max);
cab_seg_h     = cab_h / cab_seg_n;
mech_seg_i    = floor(mech_z / cab_seg_h);
mech_local_z  = mech_z - mech_seg_i * cab_seg_h - cab_seg_h / 2;
