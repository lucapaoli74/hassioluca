// Mobile esterno — pannelli piatti quotati, con veri punti di ancoraggio al
// meccanismo (non solo un guscio a misura):
//
//  - i pannelli laterali portano il cerchio di fori dell'housing (stessi
//    mount_ear_r/mount_hole_r di housing.scad — vedi params.scad): housing
//    si avvita LÌ, non "da qualche parte dentro alla scatola"
//  - il pannello laterale "drive" ha in più il pattern NEMA17 e il foro
//    passaggio albero, per il motore avvitato da fuori
//  - il pannello superiore ha i 4 fori d'angolo che combaciano col bordo
//    della tramoggia (hopper.scad), oltre al ritaglio per farla sporgere
//
// Dimensionati per stare dentro al piano di stampa di una Bambu X1C
// (256x256x256mm, vedi x1c_max in params.scad): i pannelli sopra/sotto/
// fronte sono più piccoli del piano, quelli laterali/posteriore sono più
// alti quindi spezzati in `cab_seg_n` segmenti — il segmento di giunzione è
// scelto apposta sopra al cerchio di fori dell'housing, vedi mech_seg_i.
//
// GIUNZIONI — niente più staffe/M4/dadi, solo stampa+colla dove il pezzo
// resta fisso, viti stampate dove deve restare smontabile:
//  - fianco<->fianco (segmento sotto/sopra) e retro<->retro: giunto a
//    pettine (finger_teeth, helpers.scad), incollato — molta più superficie
//    di contatto della sola fila di viti, quindi niente listello di rinforzo
//  - fianco<->retro (i 4 spigoli verticali): una linguetta stampata su ogni
//    segmento di fianco che entra in una tasca cieca sul retro — centra i
//    pezzi mentre la colla fa presa, la tenuta vera è la colla sulla
//    superficie di contatto
//  - sopra/sotto: appoggiano a filo sui bordi della pila di pareti (già
//    allineati dagli spigoli di cui sopra) e si incollano lì, senza
//    ulteriore incastro
//  - housing<->fianchi, tramoggia<->sopra, pannello frontale<->fianchi:
//    RESTANO smontabili (serve accesso a motore/rullo/tramoggia) — vite
//    stampata (printed_screw, helpers.scad) in una bocchetta filettata
//    sull'altro pezzo, niente dado
//
// Il fronte del mobile È front_panel.scad (già con feritoia/pulsante/LED,
// e già sotto i 250mm: non spezzato).
//
// Layout interno: vedi lo schema di assi/quote in cima a params.scad e
// l'anteprima 3D in assembly.scad.
include <params.scad>
include <helpers.scad>

hopper_cut_w = hopper_top_w + 2;   // ritaglio nel pannello superiore
hopper_cut_d = hopper_top_d + 2;
// il ritaglio è centrato sulla stessa Y del rullo/housing (mech_y): la
// tramoggia sta sopra di loro, non "verso il retro" a caso
hopper_cut_cy = mech_y - cab_d/2;   // offset dal centro pannello (Y=cab_d/2)

// hopper_corner_x/y sono ora in params.scad (condivisi con hopper.scad)

vent_slot_w = 40;
vent_slot_h = 5;

module panel_2d(w, h) {
    square([w, h], center = true);
}

// cerchio di fori per l'housing (stesso pattern di mount_ears in
// housing.scad): 4 fori di passaggio a mount_hole_r (la filettatura sta
// sull'orecchietta dell'housing, qui c'è solo il foro libero per la vite),
// più un foro centrale per l'albero/accoppiatore. bore_d va passato diverso
// lato drive/idler.
module housing_mount_holes(cx, cy, bore_d) {
    translate([cx, cy]) {
        circle(d = bore_d, $fn = 48);
        for (a = mount_angles)
            rotate([0, 0, a])
                translate([mount_hole_r, 0])
                    circle(d = printed_screw_clear_d, $fn = 24);
    }
}

// --- Giunto a pettine tra segmento inferiore/superiore ----------------------
// Vedi finger_teeth in helpers.scad: alla fase 0 il segmento inferiore
// aggiunge un dente (entra nel superiore) e il superiore taglia una tacca
// alla stessa fase per fargli posto; alla fase 1 è il contrario. length/fw/
// depth uguali su entrambi i lati così i denti coincidono esattamente. Ogni
// pannello chiama la coppia ADD/CUT giusta per il proprio bordo — ADD va
// nella union() che costruisce il pannello, CUT nella difference() che lo
// rifinisce (sono due operazioni diverse, non si possono unire in un solo
// modulo "combinato").
// Ognuno di questi 4 moduli disegna centrato in X e relativo a y=0 = quota
// del bordo: il chiamante fa translate([0, ±cab_seg_h/2]) alla quota vera
// del bordo prima di invocarli.
module seam_finger_add_top(length) {
    translate([-length/2, 0]) finger_teeth(length, finger_w, finger_depth, 0);   // sporge in su (y: 0..+depth)
}
module seam_finger_cut_top(length) {
    translate([-length/2, -finger_depth]) finger_teeth(length, finger_w, finger_depth, 1); // tacca in giù (y: -depth..0)
}
module seam_finger_add_bottom(length) {
    translate([-length/2, -finger_depth]) finger_teeth(length, finger_w, finger_depth, 1); // sporge in giù (y: -depth..0)
}
module seam_finger_cut_bottom(length) {
    translate([-length/2, 0]) finger_teeth(length, finger_w, finger_depth, 0);   // tacca in su (y: 0..+depth)
}

// --- Linguette/tasche di centraggio fianco<->retro --------------------------
// Posizioni in altezza LOCALE al segmento (ly, y=0 al centro del segmento),
// scelte per restare lontane da fori housing/NEMA17/vent quando il segmento
// li porta (has_mount) — verificato leggendo mech_local_z/mount_hole_r,
// non a occhio: il cerchio fori housing in questo segmento arriva fino a
// ly = mech_local_z + mount_hole_r + 2 ≈ 50.7, quindi le linguette qui
// stanno sotto, non sopra.
dowel_ly_mount   = [-60, -18];  // segmento con l'housing (mech_seg_i)
dowel_ly_open    = [-50, 50];   // segmento senza vincoli

function dowel_positions(has_mount) = has_mount ? dowel_ly_mount : dowel_ly_open;

// --- Vite di rinforzo agli spigoli verticali fianco<->retro -----------------
// In aggiunta alla linguetta/colla: una vite stampata vera e propria, per
// spigolo, che attraversa il bordo posteriore del fianco e si infila in un
// foro filettato sul retro. Quota scelta lontana dalle linguette (che
// restano a ±18/±60 o ±50) e dal cerchio fori housing (che nel segmento
// has_mount arriva fino a ~50.7): -40 sta a metà tra le due linguette del
// segmento housing, 0 sta a metà tra le due linguette del segmento libero.
// NOTA: questo rinforza lo SPIGOLO (fianco<->retro), non la giunzione a
// pettine tra segmenti sovrapposti (fianco sopra<->fianco sotto) — quella è
// un incastro complanare, vedi la nota in side_panel_seg_3d() più sotto per
// il perché lì una vite dritta non può funzionare.
function corner_screw_ly(has_mount) = has_mount ? -40 : 0;

// linguetta: sporge dal bordo POSTERIORE del fianco (x locale = +cab_d/2)
// per dowel_h, alta dowel_d, sulle quote date — un'estrusione lungo Z (lo
// spessore pannello) la rende un blocchetto a tutto spessore, non serve
// altro che questa aggiunta 2D
module back_edge_dowels(ly_list) {
    for (ly = ly_list)
        translate([cab_d/2, ly - dowel_d/2])
            square([dowel_h, dowel_d]);
}

// --- Pannello superiore: ritaglio + fori (passanti) per il bordo tramoggia -
// I 4 fori d'angolo per la tramoggia (D) restano PIENI qui: sono filettati
// (vite stampata), non un semplice foro passante — vedi top_panel_3d().
module top_panel() {
    difference() {
        panel_2d(cab_w, cab_d);
        translate([0, hopper_cut_cy])
            square([hopper_cut_w, hopper_cut_d], center = true);
    }
}

// --- Pannello inferiore: nessun foro (si incolla a filo sulla pila pareti) -
module bottom_panel() {
    panel_2d(cab_w, cab_d);
}

// --- Pannello laterale, segmento i (0 = in basso) --------------------------
// is_drive=true -> lato motore (foro passaggio + pattern NEMA17 esterno)
// is_drive=false -> lato folle (solo foro di passaggio perno)
module side_panel_seg(i, is_drive) {
    is_bottom = (i == 0);
    is_top = (i == cab_seg_n - 1);
    has_mount = (i == mech_seg_i);
    n = 4;
    usable = cab_d - 40;
    pitch = usable / n;
    difference() {
        union() {
            panel_2d(cab_d, cab_seg_h);
            if (!is_top)    translate([0, cab_seg_h/2])  seam_finger_add_top(cab_d);
            if (!is_bottom) translate([0, -cab_seg_h/2]) seam_finger_add_bottom(cab_d);
            back_edge_dowels(dowel_positions(has_mount));
        }
        if (!is_top)    translate([0, cab_seg_h/2])  seam_finger_cut_top(cab_d);
        if (!is_bottom) translate([0, -cab_seg_h/2]) seam_finger_cut_bottom(cab_d);
        if (is_bottom)
            for (k = [0:n-1])
                translate([-usable/2 + pitch/2 + k * pitch, -cab_seg_h/2 + 40])
                    square([min(vent_slot_w, pitch - 8), vent_slot_h], center = true);
        if (has_mount) {
            // mech_y è centrato su cab_d/2, cioè sull'origine di questo
            // pannello (X locale = Y globale - cab_d/2 = 0)
            bore = is_drive ? nema17_shaft_bore_d : idler_shaft_clear_d;
            housing_mount_holes(mech_y - cab_d/2, mech_local_z, bore);
            if (is_drive)
                for (dx = [-nema17_hole_spacing/2, nema17_hole_spacing/2])
                    for (dz = [-nema17_hole_spacing/2, nema17_hole_spacing/2])
                        translate([mech_y - cab_d/2 + dx, mech_local_z + dz])
                            circle(d = nema17_hole_d, $fn = 16);
        }
        // NOTA: l'aggancio col pannello frontale (front_panel.scad) non è
        // un foro in questo profilo 2D — la vite entra di taglio nel bordo
        // anteriore, non attraverso lo spessore, quindi è una lavorazione
        // solo-3D: vedi side_panel_seg_3d() più sotto.
    }
}

// --- Pannello posteriore, segmento i (0 = in basso) -------------------------
module back_panel_seg(i) {
    is_bottom = (i == 0);
    is_top = (i == cab_seg_n - 1);
    difference() {
        union() {
            panel_2d(cab_w, cab_seg_h);
            if (!is_top)    translate([0, cab_seg_h/2])  seam_finger_add_top(cab_w);
            if (!is_bottom) translate([0, -cab_seg_h/2]) seam_finger_add_bottom(cab_w);
        }
        if (!is_top)    translate([0, cab_seg_h/2])  seam_finger_cut_top(cab_w);
        if (!is_bottom) translate([0, -cab_seg_h/2]) seam_finger_cut_bottom(cab_w);
        if (is_bottom) translate([0, -cab_seg_h/2 + 30]) circle(d = 20, $fn = 32);
    }
}

// --- Versioni 3D con lettera identificativa (vedi assembly-guide.md) -------
// I moduli sopra restano 2D apposta (validi per l'export .dxf da taglio);
// queste versioni li estrudono a panel_mat_t, incidono la lettera e
// aggiungono le lavorazioni che hanno senso solo in 3D (svasatura per le
// teste delle viti stampate, tasche di centraggio, piedini) — usale per
// l'export STL/3MF da stampa, non cut_layout() che resta solo per il
// taglio piatto. Posizioni verificate per le quote di default: se cambi
// molto i parametri, ricontrolla con un render che niente si sovrapponga.

// svasatura per la testa esagonale della vite stampata, sulla faccia
// ESTERNA (z=panel_mat_t) — praticala DOPO aver fatto l'extrude del
// pannello 2D (che ha già il foro di passaggio pieno spessore)
module cbore(x, y) {
    translate([x, y, panel_mat_t - printed_screw_counterbore_h + 0.01])
        cylinder(d = printed_screw_counterbore_d, h = printed_screw_counterbore_h, $fn = 32);
}

module top_panel_3d() {
    difference() {
        linear_extrude(height = panel_mat_t) top_panel();
        label_cut("H", 0, (hopper_cut_d/2 + cab_d/2) / 2, panel_mat_t);
        // filettatura per la vite che scende dall'angolo del bordo
        // tramoggia (D) — qui niente svasatura: la testa della vite resta
        // sopra, appoggiata su D (vedi hopper.scad), non incassata in H
        for (x = [-hopper_corner_x, hopper_corner_x])
            for (y = [hopper_cut_cy - hopper_corner_y, hopper_cut_cy + hopper_corner_y])
                translate([x, y, -0.5]) printed_screw_hole(panel_mat_t + 1);
    }
}

// piedino: un tronco di piramide basso, stampato in un pezzo col pannello
// (nessun piedino di gomma da incollare — se vuoi più presa, una gocciolina
// di gomma siliconica sopra va comunque bene, ma non è necessaria)
module foot(h = 4, base = 14, top = 9) {
    hull() {
        cylinder(d = base, h = 0.01, $fn = 24);
        translate([0, 0, h]) cylinder(d = top, h = 0.01, $fn = 24);
    }
}

module bottom_panel_3d() {
    difference() {
        union() {
            linear_extrude(height = panel_mat_t) bottom_panel();
            for (x = [-cab_w/2 + 18, cab_w/2 - 18])
                for (y = [-cab_d/2 + 18, cab_d/2 - 18])
                    translate([x, y, -4]) foot();
        }
        label_cut("I", 0, 0, panel_mat_t);
    }
}

module side_panel_seg_3d(i, is_drive, letter, lx, ly) {
    has_mount = (i == mech_seg_i);
    is_bottom = (i == 0);
    mount_cx = mech_y - cab_d/2;
    mount_cy = mech_local_z;
    difference() {
        linear_extrude(height = panel_mat_t) side_panel_seg(i, is_drive);
        label_cut(letter, lx, ly, panel_mat_t);
        if (has_mount)
            for (a = mount_angles)
                cbore(mount_cx + mount_hole_r * cos(a), mount_cy + mount_hole_r * sin(a));
        // aggancio col pannello frontale (G): un foro filettato bucato DI
        // TAGLIO nel bordo anteriore (x locale = -cab_d/2), non attraverso
        // lo spessore — la vite entra dalla faccia esterna di G e s'infila
        // qui in orizzontale. Centrato nello spessore pannello (z =
        // panel_mat_t/2); rotate([0,90,0]) manda l'estrusione di
        // printed_screw_hole (di default lungo +Z) lungo +X locale —
        // verificato con lo stesso ragionamento di label_cut_y/label_tag.
        if (is_bottom)
            for (gz = [10, front_panel_h - 10])
                translate([-cab_d/2 - 0.5, gz - cab_seg_h/2, panel_mat_t/2])
                    rotate([0, 90, 0])
                        printed_screw_hole(8.5);
        // vite di rinforzo spigolo fianco<->retro, bordo POSTERIORE (x
        // locale = +cab_d/2, opposto al foro G sopra). rotate([0,-90,0])
        // manda l'estrusione nativa (lungo +Z) lungo -X locale — per punti:
        // ruotando (0,0,z) attorno a Y di -90° si ottiene (-z,0,0), quindi
        // un cieco che entra DAL bordo posteriore verso l'interno, verso
        // -X, esattamente come serve qui (stesso ragionamento della G, sul
        // bordo opposto)
        translate([cab_d/2 + 0.5, corner_screw_ly(has_mount), panel_mat_t/2])
            rotate([0, -90, 0])
                printed_screw_hole(8.5);
    }
}

module back_panel_seg_3d(i, letter, lx, ly) {
    difference() {
        union() {
            linear_extrude(height = panel_mat_t) back_panel_seg(i);
        }
        label_cut(letter, lx, ly, panel_mat_t);
        // tasche di centraggio per le linguette dei due fianchi (sinistro
        // = drive, destro = idler) — cieche, profonde dowel_h+gioco, tagliate
        // dalla faccia INTERNA (z=0, quella rivolta verso l'interno del
        // mobile, dove i fianchi la toccano)
        has_mount = (i == mech_seg_i);
        for (ly_d = dowel_positions(has_mount)) {
            translate([-cab_w/2 - 0.5, -ly_d - dowel_d/2 - dowel_clearance/2, -0.5])
                cube([panel_mat_t + 1, dowel_d + dowel_clearance, dowel_h + dowel_clearance + 0.5]);
            translate([cab_w/2 - panel_mat_t - 0.5, -ly_d - dowel_d/2 - dowel_clearance/2, -0.5])
                cube([panel_mat_t + 1, dowel_d + dowel_clearance, dowel_h + dowel_clearance + 0.5]);
        }
        // vite di rinforzo spigolo: foro di passaggio passante + svasatura
        // per la testa, sulla faccia esterna (z=panel_mat_t) — un punto per
        // fianco (drive a -X, idler a +X), stessa quota Y del foro
        // filettato sul fianco ma con segno invertito (stessa relazione
        // ly_N=-ly_J già usata sopra per le tasche linguetta/dowel)
        corner_y = -corner_screw_ly(has_mount);
        for (corner_x = [-(cab_w/2 - panel_mat_t/2), cab_w/2 - panel_mat_t/2]) {
            translate([corner_x, corner_y, -0.5])
                cylinder(d = printed_screw_clear_d, h = panel_mat_t + 1, $fn = 24);
            cbore(corner_x, corner_y);
        }
    }
}

// Layout piatto per stampa/taglio (tutti i pannelli affiancati, scala 1:1).
// Ogni pezzo sta singolarmente dentro il piano X1C da 256x256 — verificalo
// tu stesso se cambi i parametri, con `openscad -o check.dxf`.
module cut_layout() {
    gap = 15;
    translate([0, 0]) top_panel();
    translate([cab_w + gap, 0]) bottom_panel();
    row2_y = -cab_d/2 - gap - cab_seg_h * cab_seg_n - gap * cab_seg_n;
    for (i = [0:cab_seg_n-1])
        translate([0, row2_y - i * (cab_seg_h + gap)]) rotate([0,0,90]) side_panel_seg(i, true);
    for (i = [0:cab_seg_n-1])
        translate([cab_d + gap, row2_y - i * (cab_seg_h + gap)]) rotate([0,0,90]) side_panel_seg(i, false);
    for (i = [0:cab_seg_n-1])
        translate([2*(cab_d + gap), row2_y - i * (cab_seg_h + gap)]) rotate([0,0,90]) back_panel_seg(i);
}

cut_layout();
