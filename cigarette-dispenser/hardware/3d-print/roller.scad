// Rullo scanalato (singolarizzatore) — stampa 1x.
// Progettato con l'asse di rotazione lungo Z (come i vecchi file del
// carosello) per riusare helpers.scad; in assembly.scad viene ruotato di
// 90° per giacere orizzontale, immerso nella tramoggia.
//
// Ogni scanalatura è un canale cilindrico lungo tutta la lunghezza del
// rullo, dimensionato per UNA sola sigaretta (troppo stretto perché due ci
// stiano affiancate — vedi il controllo in params.scad: groove_d < 2*cig_d).
include <params.scad>
include <helpers.scad>

// Lettera "A" (vedi assembly-guide.md): incisa sulla faccia piatta di
// un'estremità, nell'anello pieno tra il foro albero (r=shaft_d/2) e il
// fondo delle scanalature (r=core_r) — l'unica zona senza tagli.
module roller() {
    difference() {
        cylinder(r = roller_r, h = roller_len, $fn = 96);
        for (i = [0 : flutes - 1])
            rotate([0, 0, i * 360 / flutes])
                translate([r_pitch, 0, -1])
                    cylinder(r = groove_d/2, h = roller_len + 2, $fn = 32);
        // foro per lo stelo dell'accoppiatore stampato (coupler.scad),
        // lato motore = Z0 (vedi housing.scad: drive_end_plate sta a
        // Z negativo, prima di questo rullo)
        translate([0, 0, -1])
            cylinder(d = shaft_d, h = roller_len + 2, $fn = 32);
        // foro per la spina di bloccaggio (spezzone di filo/chiodo 3mm, o
        // una spina stampata) — a roller_pin_z dal lato motore, dentro la
        // profondità d'innesto dell'accoppiatore (coupler_roller_len)
        translate([-roller_r - 1, 0, roller_pin_z])
            rotate([0, 90, 0])
                cylinder(d = coupler_pin_d, h = roller_r*2 + 2, $fn = 16);
        label_cut("A", (shaft_d/2 + core_r)/2, 0, roller_len);
    }
}

roller();
