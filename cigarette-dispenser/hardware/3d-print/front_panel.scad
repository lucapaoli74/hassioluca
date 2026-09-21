// Pannello frontale — stampa 1x (o taglialo da un pannello piatto:
// legno/plexiglass/alluminio, usando questo file solo come dima/misure).
// Copre solo la parte bassa del mobile (rullo/scivolo): la tramoggia sopra
// resta chiusa dalle sue stesse pareti, quindi questo pannello sta sotto i
// 250mm e non va spezzato per la stampa. Ospita: pulsante di erogazione,
// LED di stato, feritoia di uscita allineata allo scivolo (chute.scad).
//
// SICUREZZA: la feritoia deve restare lunga quanto la sigaretta (non si
// può restringere sotto quella misura, è un vincolo fisico del prodotto),
// quindi da sola NON impedisce di infilare una mano — è lo scivolo dietro
// di essa (le alette sfalsate in chute.scad) a impedire di raggiungere in
// linea retta il rullo. Qui riduciamo solo la larghezza al minimo utile.
//
// Stessa convenzione di assi del mobile (params.scad): X = larghezza
// (centrata su 0), Z = altezza da terra, Y = spessore/profondità.
include <params.scad>
include <helpers.scad>

panel_w = cab_w;
panel_h = front_panel_h;
panel_t = 4;

button_d   = 16;   // pulsante momentaneo antivandalo 16mm, foro standard
led_d      = 8;
slot_w     = drop_w + 8;       // un po' più larga dell'uscita dello scivolo
slot_h     = cig_d + 6;        // stretta il giusto per far uscire la sigaretta
slot_z     = chute_bottom_z;   // allineata all'uscita reale dello scivolo
button_z   = panel_h * 0.55;
led_z      = button_z + 30;

// fori di fissaggio ai fianchi (J/L): centrati sullo spessore dei pannelli
// laterali (non sul loro bordo — un foro esattamente sul bordo manca il
// materiale), vedi side_panel_seg_3d() in cabinet.scad per il foro
// filettato corrispondente, bucato di taglio nel bordo anteriore del fianco
fix_x = cab_w/2 - panel_mat_t/2;

module front_panel() {
    difference() {
        translate([-panel_w/2, 0, 0])
            cube([panel_w, panel_t, panel_h]);
        // feritoia di uscita, allineata allo scivolo
        translate([0, -1, slot_z])
            rotate([-90, 0, 0])
                linear_extrude(height = panel_t + 2)
                    square([slot_w, slot_h], center = true);
        // pulsante
        translate([0, -1, button_z])
            rotate([-90, 0, 0])
                cylinder(d = button_d, h = panel_t + 2, $fn = 32);
        // LED di stato, sopra al pulsante
        translate([0, -1, led_z])
            rotate([-90, 0, 0])
                cylinder(d = led_d, h = panel_t + 2, $fn = 24);
        // fori di fissaggio: passaggio vite stampata (helpers.scad) verso
        // il fianco, con svasatura per la testa esagonale sulla faccia
        // esterna (y=0, quella vista dall'utente) così non sporge
        for (x = [-fix_x, fix_x])
            for (z = [10, panel_h - 10]) {
                translate([x, -1, z])
                    rotate([-90, 0, 0])
                        cylinder(d = printed_screw_clear_d, h = panel_t + 2, $fn = 24);
                translate([x, -0.01, z])
                    rotate([-90, 0, 0])
                        cylinder(d = printed_screw_counterbore_d,
                                 h = printed_screw_counterbore_h + 0.01, $fn = 32);
            }
        // Lettera "G" (vedi assembly-guide.md): incisa sulla faccia interna
        // (verso il mobile, y = panel_t), tra la feritoia e il pulsante
        label_cut_y("G", -30, panel_t, 30, size = 7);
    }
}

// La geometria nativa sopra ha Z = altezza pannello (135mm), Y = spessore
// (4mm) — corretto per come i fori sono tagliati (rotate([-90,0,0]) al
// loro interno), ma se esportato così com'è il pezzo stamperebbe DRITTO
// IN PIEDI (135mm di parete sottile non supportata), non disteso come
// tutti gli altri pannelli del mobile. Per l'export STL/3MF va disteso:
// stessa rotazione usata per i fori, applicata a tutto il pezzo, con una
// translate di compensazione perché lo spessore (ex-Y, ora Z) resti in
// [0, panel_t] invece di [-panel_t, 0] — verificato per punti:
// rotate([-90,0,0]) manda (x,y,z) -> (x,z,-y), poi +panel_t su z riporta
// il range a positivo.
translate([0, 0, panel_t]) rotate([-90, 0, 0]) front_panel();
