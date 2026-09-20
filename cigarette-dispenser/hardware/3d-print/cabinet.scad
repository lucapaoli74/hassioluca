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
// alti quindi spezzati in `cab_seg_n` segmenti che si avvitano insieme
// lungo la giunzione (fori M4 allineati) — il segmento di giunzione è
// scelto apposta sopra al cerchio di fori dell'housing, vedi mech_seg_i.
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

hopper_corner_x = hopper_top_w/2 + rim_w/2;
hopper_corner_y = hopper_top_d/2 + rim_w/2;

vent_slot_w = 40;
vent_slot_h = 5;

module panel_2d(w, h) {
    square([w, h], center = true);
}

// fori M4 lungo un bordo orizzontale, alle stesse ascisse su entrambi i
// pezzi da unire — così combaciano quando i segmenti vengono impilati
module seam_holes(width, y) {
    usable = width - 30;
    for (i = [0:seam_hole_n - 1])
        translate([-usable/2 + i * usable / (seam_hole_n - 1), y])
            circle(d = seam_hole_d, $fn = 16);
}

// cerchio di fori per l'housing (stesso pattern di mount_ears in
// housing.scad): 4 fori a mount_hole_r, più un foro centrale per
// l'albero/accoppiatore. bore_d va passato diverso lato drive/idler.
module housing_mount_holes(cx, cy, bore_d) {
    translate([cx, cy]) {
        circle(d = bore_d, $fn = 48);
        for (a = mount_angles)
            rotate([0, 0, a])
                translate([mount_hole_r, 0])
                    circle(d = mount_hole_d, $fn = 16);
    }
}

// --- Pannello superiore: ritaglio + fori d'angolo per il bordo tramoggia -
module top_panel() {
    difference() {
        panel_2d(cab_w, cab_d);
        translate([0, hopper_cut_cy])
            square([hopper_cut_w, hopper_cut_d], center = true);
        for (x = [-hopper_corner_x, hopper_corner_x])
            for (y = [hopper_cut_cy - hopper_corner_y, hopper_cut_cy + hopper_corner_y])
                translate([x, y]) circle(d = 4, $fn = 16);
        // fori di fissaggio agli angoli del pannello
        for (x = [-cab_w/2 + 12, cab_w/2 - 12])
            for (y = [-cab_d/2 + 12, cab_d/2 - 12])
                translate([x, y]) circle(d = 4, $fn = 16);
    }
}

// --- Pannello inferiore: solo fori per piedini/staffe ----------------------
module bottom_panel() {
    difference() {
        panel_2d(cab_w, cab_d);
        for (x = [-cab_w/2 + 15, cab_w/2 - 15])
            for (y = [-cab_d/2 + 15, cab_d/2 - 15])
                translate([x, y]) circle(d = 5, $fn = 16);
    }
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
        panel_2d(cab_d, cab_seg_h);
        if (is_bottom)
            for (k = [0:n-1])
                translate([-usable/2 + pitch/2 + k * pitch, -cab_seg_h/2 + 40])
                    square([min(vent_slot_w, pitch - 8), vent_slot_h], center = true);
        if (!is_top) seam_holes(cab_d, cab_seg_h/2 - 10);
        if (!is_bottom) seam_holes(cab_d, -cab_seg_h/2 + 10);
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
    }
}

// --- Pannello posteriore, segmento i (0 = in basso) -------------------------
module back_panel_seg(i) {
    is_bottom = (i == 0);
    is_top = (i == cab_seg_n - 1);
    difference() {
        panel_2d(cab_w, cab_seg_h);
        if (is_bottom) translate([0, -cab_seg_h/2 + 30]) circle(d = 20, $fn = 32);
        if (!is_top) seam_holes(cab_w, cab_seg_h/2 - 10);
        if (!is_bottom) seam_holes(cab_w, -cab_seg_h/2 + 10);
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
