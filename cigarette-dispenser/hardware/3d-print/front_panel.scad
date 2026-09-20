// Pannello frontale — stampa 1x (o taglialo da un pannello piatto:
// legno/plexiglass/alluminio, usando questo file solo come dima/misure).
// Ospita: pulsante di erogazione, LED di stato, feritoia di uscita
// sigaretta allineata allo scivolo (chute.scad).
//
// SICUREZZA: la feritoia deve restare lunga quanto la sigaretta (non si
// può restringere sotto quella misura, è un vincolo fisico del prodotto),
// quindi da sola NON impedisce di infilare una mano — è lo scivolo dietro
// di essa (le alette sfalsate in chute.scad) a impedire di raggiungere in
// linea retta il rullo. Qui riduciamo solo la larghezza al minimo utile.
include <params.scad>

panel_w = 160;
panel_h = 200;
panel_t = 4;

button_d   = 16;   // pulsante momentaneo antivandalo 16mm, foro standard
led_d      = 8;
slot_w     = roller_len + 8;   // un po' più larga dell'uscita dello scivolo
slot_h     = cig_d + 6;        // stretta il giusto per far uscire la sigaretta

module front_panel() {
    difference() {
        cube([panel_w, panel_h, panel_t]);
        // feritoia di uscita, in basso, centrata
        translate([panel_w/2, slot_h/2 + 15, -1])
            linear_extrude(height = panel_t + 2)
                square([slot_w, slot_h], center = true);
        // pulsante, al centro
        translate([panel_w/2, panel_h/2, -1])
            cylinder(d = button_d, h = panel_t + 2, $fn = 32);
        // LED di stato, sopra al pulsante
        translate([panel_w/2, panel_h/2 + 30, -1])
            cylinder(d = led_d, h = panel_t + 2, $fn = 24);
        // fori di fissaggio agli angoli
        for (x = [10, panel_w - 10])
            for (y = [10, panel_h - 10])
                translate([x, y, -1])
                    cylinder(d = 4, h = panel_t + 2, $fn = 16);
    }
}

front_panel();
