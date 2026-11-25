// Staffa a 90° per il sensore meteo Ecowitt WS90, da fissare a un palo
// (es. palo telefonico in legno Ø 15-30 cm) con due fascette inox.
//
//   piastra con incavo a V  -> si appoggia al palo, si adatta a diametri diversi
//   2 asole                 -> per reggetta inox / fascette fino a 22 mm
//   braccio a 90° + rinforzo
//   manicotto verticale     -> ci entra un tubo Ø 35 mm (spezzone di palo antenna);
//                              il WS90 si stringe in cima al tubo con la sua staffa
//   2 viti M6 + dado        -> bloccano il tubo nel manicotto
//
// Stampa (Bambu Lab H2D): file STL già orientato, coricato su un fianco.
// ASA (o PETG), 6 pareti, 6 strati sopra/sotto, riempimento gyroid 40%,
// nessun supporto. Misure in mm.

/* [Misure principali] */
tubo_d      = 35;   // diametro del tubo che regge il WS90
gioco       = 0.6;  // gioco del foro sul tubo
braccio     = 230;  // dal palo all'asse del manicotto
larghezza   = 56;   // spessore della staffa (asse Y)
piastra_h   = 160;  // altezza della piastra sul palo
braccio_h   = 45;   // altezza del braccio
manicotto_h = 85;   // altezza del manicotto
asola_l     = 22;   // larghezza fascetta (+ gioco)
asola_s     = 5;    // spessore fascetta (+ gioco)

/* [Vista] */
vista = "stampa"; // [stampa, montata]

$fn = 72;
piastra_s = 14;                       // spessore piastra dietro la V
v_prof    = (larghezza / 2) / tan(60); // profondità della V a 120°
x0        = -(piastra_s + v_prof);    // lato palo
man_d     = tubo_d + gioco + 2 * 7;   // diametro esterno manicotto
z_br      = piastra_h - braccio_h;    // quota sotto del braccio

module piastra() {
    difference() {
        translate([x0, -larghezza / 2, 0]) cube([-x0, larghezza, piastra_h]);
        // incavo a V verso il palo
        translate([x0, 0, -1]) linear_extrude(piastra_h + 2)
            polygon([[-0.01, -larghezza / 2 - 0.01], [v_prof, 0], [-0.01, larghezza / 2 + 0.01]]);
        // asole per le fascette (attraversano da un fianco all'altro)
        for (z = [22, piastra_h - 22])
            translate([x0 + v_prof + 3, -larghezza / 2 - 1, z - asola_l / 2])
                cube([asola_s, larghezza + 2, asola_l]);
    }
}

module braccio_r() {
    translate([0, -larghezza / 2, z_br]) cube([braccio, larghezza, braccio_h]);
    // rinforzo a 45° sotto il braccio
    hull() {
        translate([-0.01, -larghezza / 2, 8]) cube([0.01, larghezza, 1]);
        translate([-0.01, -larghezza / 2, z_br]) cube([0.01, larghezza, 1]);
        translate([z_br - 8, -larghezza / 2, z_br]) cube([0.01, larghezza, 1]);
    }
}

module manicotto_pieno() {
    translate([braccio, 0, z_br]) cylinder(d = man_d, h = manicotto_h + braccio_h);
    // borchie per i dadi M6
    for (z = [z_br + braccio_h + 18, z_br + braccio_h + manicotto_h - 18])
        translate([braccio, 0, z]) rotate([0, 90, 0]) cylinder(d = 20, h = man_d / 2 + 6);
}

module fori_manicotto() {
    // foro per il tubo (cieco, con scarico acqua)
    translate([braccio, 0, z_br + 10]) cylinder(d = tubo_d + gioco, h = manicotto_h + braccio_h);
    translate([braccio, 0, z_br - 1]) cylinder(d = 6, h = 12);
    // viti M6 con dado incassato
    for (z = [z_br + braccio_h + 18, z_br + braccio_h + manicotto_h - 18])
        translate([braccio, 0, z]) rotate([0, 90, 0]) {
            cylinder(d = 6.5, h = man_d / 2 + 10);
            translate([0, 0, man_d / 2 + 6 - 5.5]) rotate([0, 0, 30]) cylinder(d = 11.6, h = 10, $fn = 6);
        }
}

module staffa() {
    difference() {
        union() { piastra(); braccio_r(); manicotto_pieno(); }
        fori_manicotto();
        // smussi sugli spigoli esterni del braccio
        for (s = [-1, 1]) translate([braccio / 2, s * larghezza / 2, piastra_h])
            rotate([45, 0, 0]) cube([braccio + 60, 4, 4], center = true);
    }
}

if (vista == "montata") staffa();
else rotate([90, 0, 0]) translate([0, larghezza / 2, 0]) staffa(); // coricata sul fianco
