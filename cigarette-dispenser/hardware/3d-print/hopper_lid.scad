// Coperchio della tramoggia — stampa 1x. Si appoggia sul bordo aggiunto in
// hopper.scad (hopper_rim), con un labbro che scende dentro l'apertura
// per allinearsi e non lasciare fessure per le dita. Si apre con una
// cerniera comune (non stampata: vedi bom.md) fissata sul lato fronte, e
// si blocca con la serratura elettrica sul lato retro — vedi
// software/esphome (switch "Serratura tramoggia") e hardware/wiring.md
// per il sensore magnetico che conferma allo sportello di essere chiuso
// prima che il firmware autorizzi un'erogazione.
include <params.scad>
include <helpers.scad>

lid_w = hopper_top_w + 2*rim_w;
lid_d = hopper_top_d + 2*rim_w;
lid_t = 4;
skirt_h = 12;
skirt_wall = 2;

module lid_plate() {
    cube([lid_w, lid_d, lid_t], center = true);
}

module skirt() {
    // scende dentro l'apertura interna della tramoggia (dentro le pareti,
    // non dentro il bordo) per centrare il coperchio e chiudere la fessura
    inner_w = hopper_top_w - 2*hopper_wall - 1;
    inner_d = hopper_top_d - 2*hopper_wall - 1;
    translate([0, 0, -skirt_h/2])
        difference() {
            cube([inner_w, inner_d, skirt_h], center = true);
            cube([inner_w - 2*skirt_wall, inner_d - 2*skirt_wall, skirt_h + 2], center = true);
        }
}

module hinge_tab(x) {
    translate([x, -lid_d/2 + rim_w/2, 0])
        difference() {
            cylinder(d = 10, h = lid_t, center = true, $fn = 20);
            cylinder(d = hinge_hole_d, h = lid_t + 2, center = true, $fn = 16);
        }
}

module latch_catch() {
    // linguetta con foro per il gancio/perno della serratura elettrica
    translate([-10, lid_d/2 - rim_w/2, 0])
        difference() {
            cylinder(d = 12, h = lid_t, center = true, $fn = 24);
            cylinder(d = latch_hole_d + 1, h = lid_t + 2, center = true, $fn = 16);
        }
}

module hopper_lid() {
    difference() {
        union() {
            lid_plate();
            skirt();
            for (x = hinge_x) hinge_tab(x);
            latch_catch();
        }
        // Lettera "E" (vedi assembly-guide.md): angolo posteriore destro,
        // lontano da cerniere (fronte) e gancio serratura (retro sinistra)
        label_cut("E", 45, 50, lid_t/2, size = 6);
    }
}

hopper_lid();
