// Rullo scanalato (singolarizzatore) — stampa 1x.
// Progettato con l'asse di rotazione lungo Z (come i vecchi file del
// carosello) per riusare helpers.scad; in assembly.scad viene ruotato di
// 90° per giacere orizzontale, immerso nella tramoggia.
//
// Ogni scanalatura è un canale cilindrico lungo tutta la lunghezza del
// rullo, dimensionato per UNA sola sigaretta (troppo stretto perché due ci
// stiano affiancate — vedi il controllo in params.scad: groove_d < 2*cig_d).
include <params.scad>

module roller() {
    difference() {
        cylinder(r = roller_r, h = roller_len, $fn = 96);
        for (i = [0 : flutes - 1])
            rotate([0, 0, i * 360 / flutes])
                translate([r_pitch, 0, -1])
                    cylinder(r = groove_d/2, h = roller_len + 2, $fn = 32);
        // foro per il giunto dell'albero motore
        translate([0, 0, -1])
            cylinder(d = shaft_d, h = roller_len + 2, $fn = 32);
        // foro per la vite di bloccaggio del giunto
        translate([-roller_r - 1, 0, roller_len/2])
            rotate([0, 90, 0])
                cylinder(d = 3.2, h = roller_r*2 + 2, $fn = 16);
    }
}

roller();
