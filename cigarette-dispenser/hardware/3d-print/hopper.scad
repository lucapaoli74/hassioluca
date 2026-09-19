// Tramoggia (bulk hopper) — stampa 1x. Imbuto che raccoglie le sigarette
// sfuse (>=100) e le convoglia, per gravità, nell'apertura di carico
// dell'alloggiamento (housing.scad). Capacità: vedi il calcolo in
// params.scad (hopper_*).
include <params.scad>

module loft(w1, d1, w2, d2, h) {
    hull() {
        translate([0, 0, h]) linear_extrude(0.01) square([w1, d1], center = true);
        linear_extrude(0.01) square([w2, d2], center = true);
    }
}

module hopper() {
    difference() {
        loft(hopper_top_w, hopper_top_d, hopper_bot_w, hopper_bot_d, hopper_h);
        translate([0, 0, -1])
            loft(hopper_top_w - 2*hopper_wall, hopper_top_d - 2*hopper_wall,
                 hopper_bot_w - 2*hopper_wall, hopper_bot_d - 2*hopper_wall,
                 hopper_h + 2);
    }
}

hopper();
