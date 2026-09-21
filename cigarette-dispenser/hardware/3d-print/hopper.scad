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
// Parametri (rim_w, hinge_knuckle_*, ecc.) in params.scad, condivisi con
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

// 3 nocche FISSE della cerniera stampata (le altre 2, mobili, sono sul
// coperchio — hopper_lid.scad), agli slot pari 0/2/4 di un pettine a 5
// slot passo hinge_knuckle_pitch, centrato in X sul lato fronte del bordo.
// hinge_knuckle_solid/hinge_pin_cut sono in helpers.scad: stessa
// definizione usata da hopper_lid.scad con la Z corrispondente (vedi lì
// il conto che porta i due pezzi sulla stessa retta), non due fori da
// allineare a mano.
hinge_y = -hopper_top_d/2 - rim_w/2;   // Y del bordo lato fronte
hinge_z = hopper_h - rim_t/2;          // quota della cerniera su questo pezzo

module hopper() {
    union() {
        difference() {
            union() {
                loft(hopper_top_w, hopper_top_d, hopper_bot_w, hopper_bot_d, hopper_h);
                hinge_knuckle_solid(-2 * hinge_knuckle_pitch, hinge_y, hinge_z);
                hinge_knuckle_solid(0, hinge_y, hinge_z);
                hinge_knuckle_solid(2 * hinge_knuckle_pitch, hinge_y, hinge_z);
            }
            translate([0, 0, -1])
                loft(hopper_top_w - 2*hopper_wall, hopper_top_d - 2*hopper_wall,
                     hopper_bot_w - 2*hopper_wall, hopper_bot_d - 2*hopper_wall,
                     hopper_h + 2);
            hinge_pin_cut(hinge_y, hinge_z);
        }
        difference() {
            hopper_rim();
            // gancio serratura + sensore (lato retro, +Y)
            translate([-10, hopper_top_d/2 + rim_w/2, hopper_h - rim_t/2 - 0.01])
                cylinder(d = latch_hole_d, h = rim_t + 2, center = true, $fn = 16);
            translate([15, hopper_top_d/2 + rim_w/2, hopper_h - rim_t/2 - 0.01])
                cylinder(d = reed_hole_d, h = rim_t + 2, center = true, $fn = 16);
            // fori d'angolo verso il pannello superiore (H): passaggio +
            // svasatura per la testa della vite stampata, che resta QUI
            // sopra (H sotto è filettato, vedi cabinet.scad). Il bordo (il
            // "cube" in hopper_rim()) è centrato in Z su hopper_h-0.01 con
            // spessore rim_t, quindi la sua faccia superiore vera è a
            // hopper_h - 0.01 + rim_t/2, non a hopper_h.
            rim_top = hopper_h - 0.01 + rim_t/2;
            for (x = [-hopper_corner_x, hopper_corner_x])
                for (y = [-hopper_corner_y, hopper_corner_y]) {
                    translate([x, y, rim_top - rim_t - 0.5])
                        cylinder(d = printed_screw_clear_d, h = rim_t + 1, $fn = 24);
                    translate([x, y, rim_top - printed_screw_counterbore_h])
                        cylinder(d = printed_screw_counterbore_d,
                                 h = printed_screw_counterbore_h + 0.5, $fn = 32);
                }
            // Lettera "D" (vedi assembly-guide.md): angolo posteriore destro
            // del bordo, lontano da cerniera/serratura/sensore
            label_cut("D", hopper_top_w/2 + rim_w/2 - 2, hopper_top_d/2 + rim_w/2 - 2,
                       hopper_h - 0.01 + rim_t/2, size = 6);
        }
    }
}

hopper();
