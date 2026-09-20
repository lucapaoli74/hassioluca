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

id_top    = max(drop_d1 - 2*hopper_wall, 4);
id_bottom = max(drop_d2 - 2*hopper_wall, 4);
iw_top    = drop_w - 2*hopper_wall;
iw_bottom = drop_w - 6 - 2*hopper_wall;

function lerp(a, b, t) = a + (b - a) * t;

// Sbarramento anti-intrusione: due alette sfalsate (una da un lato, una
// dall'altro) impediscono a una mano infilata dalla feritoia del pannello
// di raggiungere in linea retta il rullo, sullo stesso principio di una
// buca delle lettere o di una cassaforte con imbuto a zig-zag. La
// sigaretta (piccola, cade tumbling) passa lo stesso dallo spazio lasciato
// libero sul lato opposto.
module baffle(z0, side, cover_frac, thick, overlap = 2) {
    t = z0 / drop_h;
    d = lerp(id_bottom, id_top, t);
    w = lerp(iw_bottom, iw_top, t) - 1; // piccolo gioco dalle pareti
    cov = d * cover_frac;
    // il bordo esterno affonda `overlap` nella parete (unione solida,
    // non tangente) — lo stesso accorgimento usato altrove per evitare
    // mesh non manifold da superfici coincidenti
    y_far  = side * (d/2 + overlap);
    y_near = side * (d/2 - cov);
    translate([0, (y_far + y_near) / 2, z0])
        cube([w, abs(y_far - y_near), thick], center = true);
}

module chute() {
    union() {
        difference() {
            loft(drop_w, drop_d1, drop_w - 6, drop_d2, drop_h);
            translate([0, 0, -1])
                loft(iw_top, id_top, iw_bottom, id_bottom, drop_h + 2);
        }
        // le alette si aggiungono DOPO aver scavato la cavità, altrimenti
        // la sottrazione le rimuoverebbe (sono dimensionate per stare
        // proprio dentro quello spazio cavo)
        baffle(drop_h * 0.66, 1, 0.62, 6);
        baffle(drop_h * 0.33, -1, 0.62, 6);
    }
}

chute();
