// Scivolo di scarico — stampa 1x. Si fissa sotto l'apertura di scarico
// dell'housing (lato 270°, cioè -Y in questo stesso sistema di riferimento
// locale — vedi housing.scad) e incanala la sigaretta singola verso il
// pannello frontale. La lunghezza/piega verso il pannello dipende dalla
// profondità del TUO mobile: questo pezzo è volutamente un imbuto dritto
// da adattare (accorcia/allunga `drop_h`, o uniscilo a un tubo/canalina).
include <params.scad>

drop_w  = roller_len + 6;   // combacia con l'apertura di scarico dell'housing
drop_d1 = discharge_gap_deg > 0 ? 2 * housing_or * sin(discharge_gap_deg/2) + 6 : 30;
drop_d2 = cig_d + 10;       // uscita: appena più larga della sigaretta
drop_h  = 60;

module loft(w1, d1, w2, d2, h) {
    hull() {
        translate([0, 0, h]) linear_extrude(0.01) square([w1, d1], center = true);
        linear_extrude(0.01) square([w2, d2], center = true);
    }
}

module chute() {
    difference() {
        loft(drop_w, drop_d1, drop_w - 6, drop_d2, drop_h);
        translate([0, 0, -1])
            loft(drop_w - 2*hopper_wall, max(drop_d1 - 2*hopper_wall, 4),
                 drop_w - 6 - 2*hopper_wall, max(drop_d2 - 2*hopper_wall, 4),
                 drop_h + 2);
    }
}

chute();
