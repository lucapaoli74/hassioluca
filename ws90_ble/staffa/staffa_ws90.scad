// Staffa a 90° per il sensore meteo Ecowitt WS90, da fissare a un palo
// (es. palo telefonico in legno Ø 15-30 cm) con due fascette inox.
//
//   piastra con incavo a V  -> si appoggia al palo, si adatta a diametri diversi
//   2 asole                 -> per reggetta inox / fascette fino a 22 mm
//   braccio a 90° + rinforzo
//   perno verticale maschio -> entra nell'attacco femmina sotto il WS90,
//                              che lo stringe con la sua ghiera
//   perno cavo + uscita     -> passaggio per il cavo che esce da sotto il WS90
//
// Stampa (Bambu Lab H2D): file STL già orientato, coricato su un fianco.
// ASA (o PETG), 6 pareti, 6 strati sopra/sotto, riempimento gyroid 40%,
// nessun supporto. Misure in mm.

/* [Misure principali] */
perno_d     = 23;   // diametro interno dell'attacco sotto il WS90 (misurato: 23 mm)
gioco       = 0.4;  // il perno viene più piccolo di così
perno_h     = 40;   // lunghezza del perno: più corto dell'attacco, così la ghiera stringe
cavo_d      = 12;   // foro per il cavo dentro il perno
braccio     = 230;  // dal palo all'asse del manicotto
larghezza   = 56;   // spessore della staffa (asse Y)
piastra_h   = 160;  // altezza della piastra sul palo
braccio_h   = 45;   // altezza del braccio
colletto_h  = 25;   // base del perno sopra il braccio
asola_l     = 22;   // larghezza fascetta (+ gioco)
asola_s     = 5;    // spessore fascetta (+ gioco)

/* [Vista] */
vista = "stampa"; // [stampa, montata]

$fn = 72;
piastra_s = 14;                       // spessore piastra dietro la V
v_prof    = (larghezza / 2) / tan(60); // profondità della V a 120°
x0        = -(piastra_s + v_prof);    // lato palo
man_d     = perno_d + 22;              // diametro della base del perno
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
    // base robusta sopra il braccio
    translate([braccio, 0, z_br]) cylinder(d = man_d, h = braccio_h + colletto_h);
    // perno maschio con punta smussata
    translate([braccio, 0, piastra_h + colletto_h - 0.01]) {
        cylinder(d = perno_d - gioco, h = perno_h - 2);
        translate([0, 0, perno_h - 2]) cylinder(d1 = perno_d - gioco, d2 = perno_d - gioco - 3, h = 2);
    }
}

module fori_manicotto() {
    // passaggio del cavo: dentro il perno, esce di lato sotto il braccio
    translate([braccio, 0, z_br + 12]) cylinder(d = cavo_d, h = braccio_h + colletto_h + perno_h);
    translate([braccio, 0, z_br + 12 + cavo_d / 2]) rotate([0, 90, 0]) cylinder(d = cavo_d, h = man_d);
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
