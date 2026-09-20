// Mobile esterno — pannelli piatti quotati. Dimensionati per stare dentro
// al piano di stampa di una Bambu X1C (256x256x256mm, vedi x1c_max in
// params.scad): i pannelli sopra/sotto sono più piccoli del piano, quelli
// laterali/posteriore sono più alti del piano quindi spezzati in due metà
// che si avvitano insieme lungo la giunzione (fori M4 allineati). Stampali
// piatti, o taglia la stessa sagoma in legno/alluminio composito/plexiglass
// con laser/CNC/sega se preferisci — vedi hardware/bom.md.
//
// Il fronte del mobile È front_panel.scad (già con feritoia/pulsante/LED),
// e sta da solo dentro al piano X1C senza bisogno di essere spezzato.
//
// Layout interno (dall'alto): tramoggia (sporge dal pannello superiore,
// con coperchio a chiave) -> rullo/alloggiamento -> scivolo -> pannello
// frontale con pulsante. Vedi assembly.scad per la vista 3D d'insieme.
include <params.scad>

hopper_cut_w = hopper_top_w + 2;   // ritaglio nel pannello superiore
hopper_cut_d = hopper_top_d + 2;
hopper_cut_cy = -(cab_d/2 - hopper_cut_d/2 - cab_back_margin); // verso il retro

vent_slot_w = 40;
vent_slot_h = 5;

// altezza pannelli laterali/retro: 420mm non ci sta in un piano da 250mm,
// quindi si spezza in `seg_n` segmenti impilati (ognuno <= x1c_max)
seg_n  = ceil(cab_h / x1c_max);
seg_h  = cab_h / seg_n;

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

// --- Pannello superiore: ritaglio per la tramoggia -------------------------
module top_panel() {
    difference() {
        panel_2d(cab_w, cab_d);
        translate([0, hopper_cut_cy])
            square([hopper_cut_w, hopper_cut_d], center = true);
        // fori di fissaggio agli angoli
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
// feritoie di ventilazione solo sul segmento più in basso, vicino
// all'elettronica; fori di giunzione su ogni bordo condiviso con il
// segmento adiacente
module side_panel_seg(i) {
    is_bottom = (i == 0);
    is_top = (i == seg_n - 1);
    n = 4;
    usable = cab_d - 40;
    pitch = usable / n;
    difference() {
        panel_2d(cab_d, seg_h);
        if (is_bottom)
            for (k = [0:n-1])
                translate([-usable/2 + pitch/2 + k * pitch, -seg_h/2 + 40])
                    square([min(vent_slot_w, pitch - 8), vent_slot_h], center = true);
        if (!is_top) seam_holes(cab_d, seg_h/2 - 10);
        if (!is_bottom) seam_holes(cab_d, -seg_h/2 + 10);
    }
}

// --- Pannello posteriore, segmento i (0 = in basso) -------------------------
module back_panel_seg(i) {
    is_bottom = (i == 0);
    is_top = (i == seg_n - 1);
    difference() {
        panel_2d(cab_w, seg_h);
        if (is_bottom) translate([0, -seg_h/2 + 30]) circle(d = 20, $fn = 32);
        if (!is_top) seam_holes(cab_w, seg_h/2 - 10);
        if (!is_bottom) seam_holes(cab_w, -seg_h/2 + 10);
    }
}

// Layout piatto per stampa/taglio (tutti i pannelli affiancati, scala 1:1).
// Ogni pezzo sta singolarmente dentro il piano X1C da 256x256 — verificalo
// tu stesso se cambi cab_w/cab_d/x1c_max, con `openscad -o check.dxf`.
module cut_layout() {
    gap = 15;
    translate([0, 0]) top_panel();
    translate([cab_w + gap, 0]) bottom_panel();
    row2_y = -cab_d/2 - gap - seg_h * seg_n - gap * seg_n;
    for (i = [0:seg_n-1])
        translate([0, row2_y - i * (seg_h + gap)]) rotate([0,0,90]) side_panel_seg(i);
    for (i = [0:seg_n-1])
        translate([cab_d + gap, row2_y - i * (seg_h + gap)]) rotate([0,0,90]) side_panel_seg(i);
    for (i = [0:seg_n-1])
        translate([2*(cab_d + gap), row2_y - i * (seg_h + gap)]) rotate([0,0,90]) back_panel_seg(i);
}

cut_layout();
