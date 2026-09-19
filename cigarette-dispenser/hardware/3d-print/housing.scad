// Alloggiamento a "C" del rullo — stampa 1x. Avvolge il rullo per quasi
// tutta la circonferenza per trattenere le sigarette nelle scanalature
// durante il trasporto, lasciando aperta solo la zona di carico (sotto la
// tramoggia) e quella di scarico (sopra allo scivolo). Stessa convenzione
// di assi di roller.scad: asse di rotazione lungo Z, ruotato in
// assembly.scad per giacere orizzontale.
include <params.scad>
include <helpers.scad>

drive_hole_spacing = 31;   // pattern NEMA17, M3
drive_hole_d       = 3.4;
drive_bore_d       = 10;   // passaggio giunto/albero motore
idler_bore_d       = shaft_d + 0.6;  // foro/boccola per il perno folle

module annulus(r1, r2) {
    difference() {
        circle(r = r2, $fn = 128);
        circle(r = r1, $fn = 128);
    }
}

module housing_tube() {
    linear_extrude(height = roller_len)
        difference() {
            annulus(housing_ir, housing_or);
            rotate([0, 0, 90])  angle_mask(load_gap_deg);
            rotate([0, 0, 270]) angle_mask(discharge_gap_deg);
        }
}

// orecchiette di fissaggio: sporgono radialmente dal piatto terminale, con
// foro passante lungo Z (stessa direzione dello spessore del piatto) per
// avvitare l'alloggiamento al pannello frontale/posteriore del mobile
module mount_ears(t) {
    for (a = [0, 90, 180, 270])
        rotate([0, 0, a + 45])
            translate([housing_or - 6, 0, 0])
                difference() {
                    linear_extrude(height = t)
                        translate([0, -7]) square([16, 14]);
                    translate([8, 0, -1])
                        cylinder(d = 3.4, h = t + 2, $fn = 16);
                }
}

module drive_end_plate() {
    difference() {
        union() {
            cylinder(r = housing_or, h = end_plate_t, $fn = 96);
            mount_ears(end_plate_t);
        }
        translate([0, 0, -1])
            cylinder(d = drive_bore_d, h = end_plate_t + 2, $fn = 48);
        for (i = [0:3])
            rotate([0, 0, i*90 + 45])
                translate([drive_hole_spacing/sqrt(2), 0, -1])
                    cylinder(d = drive_hole_d, h = end_plate_t + 2, $fn = 16);
    }
}

module idler_end_plate() {
    difference() {
        union() {
            cylinder(r = housing_or, h = end_plate_t, $fn = 96);
            mount_ears(end_plate_t);
        }
        translate([0, 0, -1])
            cylinder(d = idler_bore_d, h = end_plate_t + 2, $fn = 32);
    }
}

module housing() {
    union() {
        housing_tube();
        translate([0, 0, -end_plate_t]) drive_end_plate();
        translate([0, 0, roller_len]) idler_end_plate();
    }
}

housing();
