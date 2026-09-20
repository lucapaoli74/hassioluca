// Mobile esterno — pannelli piatti quotati, NON pensati per la stampa 3D
// (un guscio pieno di questa taglia sprecherebbe ore di stampa ed è più
// debole di un pannello tagliato). Taglia questi profili in legno
// multistrato 9mm, alluminio composito o plexiglass con laser/CNC/sega, e
// uniscili con angolari interni + viti (vedi hardware/bom.md).
//
// Il fronte del mobile È front_panel.scad (già con feritoia/pulsante/LED):
// tagliato o stampato, va bene lo stesso file per entrambe le tecniche.
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

module panel_2d(w, h) {
    square([w, h], center = true);
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

// --- Pannelli laterali: feritoie di ventilazione verso il basso -----------
module side_panel() {
    n = 4;
    usable = cab_d - 40; // margine di 20mm per lato
    pitch = usable / n;
    difference() {
        panel_2d(cab_d, cab_h);
        for (i = [0:n-1])
            translate([-usable/2 + pitch/2 + i * pitch, -cab_h/2 + 40])
                square([min(vent_slot_w, pitch - 8), vent_slot_h], center = true);
    }
}

// --- Pannello posteriore: passacavo -----------------------------------------
module back_panel() {
    difference() {
        panel_2d(cab_w, cab_h);
        translate([0, -cab_h/2 + 30]) circle(d = 20, $fn = 32);
    }
}

// Layout piatto per il taglio (tutti i pannelli affiancati, in scala 1:1)
module cut_layout() {
    gap = 20;
    translate([0, 0]) top_panel();
    translate([cab_w + gap, 0]) bottom_panel();
    translate([2*(cab_w + gap), 0]) rotate([0,0,90]) side_panel();
    translate([2*(cab_w + gap) + cab_h + gap, 0]) rotate([0,0,90]) side_panel();
    translate([0, -(cab_d + gap)/2 - cab_h/2 - gap]) back_panel();
}

cut_layout();
