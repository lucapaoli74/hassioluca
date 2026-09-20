// Tramoggia (bulk hopper) — stampa 1x. Imbuto che raccoglie le sigarette
// sfuse (>=100) e le convoglia, per gravità, nell'apertura di carico
// dell'alloggiamento (housing.scad). Capacità: vedi il calcolo in
// params.scad (hopper_*).
include <params.scad>
include <helpers.scad>

module loft(w1, d1, w2, d2, h) {
    hull() {
        translate([0, 0, h]) linear_extrude(0.01) square([w1, d1], center = true);
        linear_extrude(0.01) square([w2, d2], center = true);
    }
}

// bordo in cima: appoggio per il coperchio (hopper_lid.scad), sede per
// cerniera, serratura e sensore di sportello chiuso — vedi quel file.
// Parametri (rim_w, hinge_x, ecc.) in params.scad, condivisi con
// hopper_lid.scad senza dover includere questo file (che ha una hopper()
// finale — un `include` la eseguirebbe due volte).

module hopper_rim() {
    translate([0, 0, hopper_h - 0.01])
        difference() {
            union() {
                cube([hopper_top_w + 2*rim_w, hopper_top_d + 2*rim_w, rim_t], center = true);
            }
            cube([hopper_top_w, hopper_top_d, rim_t + 2], center = true);
        }
}

module hopper() {
    union() {
        difference() {
            loft(hopper_top_w, hopper_top_d, hopper_bot_w, hopper_bot_d, hopper_h);
            translate([0, 0, -1])
                loft(hopper_top_w - 2*hopper_wall, hopper_top_d - 2*hopper_wall,
                     hopper_bot_w - 2*hopper_wall, hopper_bot_d - 2*hopper_wall,
                     hopper_h + 2);
        }
        difference() {
            hopper_rim();
            // viti cerniera (lato fronte, -Y)
            for (x = hinge_x)
                translate([x, -hopper_top_d/2 - rim_w/2, hopper_h - rim_t/2 - 0.01])
                    cylinder(d = hinge_hole_d, h = rim_t + 2, center = true, $fn = 16);
            // gancio serratura + sensore (lato retro, +Y)
            translate([-10, hopper_top_d/2 + rim_w/2, hopper_h - rim_t/2 - 0.01])
                cylinder(d = latch_hole_d, h = rim_t + 2, center = true, $fn = 16);
            translate([15, hopper_top_d/2 + rim_w/2, hopper_h - rim_t/2 - 0.01])
                cylinder(d = reed_hole_d, h = rim_t + 2, center = true, $fn = 16);
            // Lettera "D" (vedi assembly-guide.md): angolo posteriore destro
            // del bordo, lontano da cerniera/serratura/sensore
            label_cut("D", hopper_top_w/2 + rim_w/2 - 2, hopper_top_d/2 + rim_w/2 - 2,
                       hopper_h - 0.01 + rim_t/2, size = 6);
        }
    }
}

hopper();
