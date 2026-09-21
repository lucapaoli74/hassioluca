// Coperchio della tramoggia — stampa 1x. Si appoggia sul bordo aggiunto in
// hopper.scad (hopper_rim), con un labbro che scende dentro l'apertura
// per allinearsi e non lasciare fessure per le dita. Si apre con una
// cerniera STAMPATA (nocche qui + 3 su hopper.scad, un perno che è solo
// uno spezzone del tuo filamento da 1.75mm — vedi helpers.scad) fissata
// sul lato fronte, e si blocca con la serratura elettrica sul lato retro —
// vedi software/esphome (switch "Serratura tramoggia") e
// hardware/wiring.md per il sensore magnetico che conferma allo sportello
// di essere chiuso prima che il firmware autorizzi un'erogazione.
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

// Nocche MOBILI della cerniera stampata (le 3 fisse sono sul bordo D, vedi
// hopper.scad): agli slot dispari 1/3 dello stesso pettine a 5 slot, così
// si incastrano tra le nocche di D invece di scontrarci.
//
// Quota Z: in assembly.scad D sta a translate z=front_panel_h, E sta a
// translate z=front_panel_h+hopper_h (nessuna rotazione su nessuno dei
// due) — per far coincidere in mondo la quota hinge_z di D (locale a D)
// con quella di E, serve E_local_z = D_hinge_z - hopper_h. Y invece è
// identica su entrambi senza conversioni (stesso offset mech_y per
// entrambi i pezzi).
hinge_y = -lid_d/2 + rim_w/2;   // = -hopper_top_d/2 - rim_w/2, stessa Y di D
hinge_z = -rim_t/2;             // = (hopper_h - rim_t/2) - hopper_h

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
            hinge_knuckle_solid(-hinge_knuckle_pitch, hinge_y, hinge_z);
            hinge_knuckle_solid(hinge_knuckle_pitch, hinge_y, hinge_z);
            latch_catch();
        }
        hinge_pin_cut(hinge_y, hinge_z);
        // Lettera "E" (vedi assembly-guide.md): angolo posteriore destro,
        // lontano da cerniere (fronte) e gancio serratura (retro sinistra)
        label_cut("E", 45, 50, lid_t/2, size = 6);
    }
}

hopper_lid();
