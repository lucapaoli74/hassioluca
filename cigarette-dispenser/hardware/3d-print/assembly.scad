// Anteprima visiva d'assieme — non da stampare. Mostra il meccanismo
// montato nel mobile con le quote reali di params.scad: se qui qualcosa si
// sovrappone o resta staccato, il bug è nei parametri, non solo
// nell'immaginazione. Apri in OpenSCAD per ruotare/zoomare.
//
// Convenzione di assi (vedi params.scad): X = larghezza (asse rullo),
// Y = profondità (0 = fronte), Z = altezza (0 = fondo mobile). Rullo e
// housing sono modellati con l'asse lungo Z "nativo": rotate([90,0,90])
// li porta a giacere lungo X con il carico rivolto verso +Z (in su) e lo
// scarico verso -Z (in giù) — verificato algebricamente (matrice di
// rotazione), non a occhio: mappa (x,y,z) nativi in (z,x,y) mondo.
include <params.scad>
use <roller.scad>
use <housing.scad>
use <grille.scad>
use <hopper.scad>
use <hopper_lid.scad>
use <chute.scad>
use <front_panel.scad>
use <cabinet.scad>

module mech_transform() {
    translate([0, mech_y, mech_z])
        rotate([90, 0, 90])
            translate([0, 0, -roller_len/2])
                children();
}

// rullo + housing, centrati in X, alla profondità/altezza mech_y/mech_z
mech_transform() {
    color("orange") roller();
    color("lightgray", 0.35) housing();
}

// griglia, appena sopra l'housing
translate([0, mech_y, front_panel_h - 1])
    color("green", 0.6) grille();

// tramoggia, appoggiata sopra la griglia
translate([0, mech_y, front_panel_h])
    color("yellow", 0.5) hopper();

translate([0, mech_y, front_panel_h + hopper_h])
    color("goldenrod", 0.8) hopper_lid();

// scivolo: il suo Y=0 nativo (in alto) aggancia lo scarico dell'housing,
// il suo fondo (Y=-chute_dy nativo) arriva davanti, vicino al pannello
translate([0, mech_y, chute_bottom_z])
    color("lightblue", 0.6) chute();

// pannello frontale: la sua origine nativa È il fronte-basso-centro del
// mobile, quindi zero traslazione
color("white") front_panel();

// --- mobile: pannelli come guscio semitrasparente, alle quote reali -----
cab_col = [0.55, 0.6, 0.66];
module panel_extrude(t) { linear_extrude(height = t) children(); }

// Rotazioni verificate algebricamente (stessa ricerca a matrice usata per
// rullo/housing): rotate([90,0,90]) manda gli assi locali del pannello
// (x=profondità, y=altezza, z=spessore) in (Y,Z,X) mondo; rotate([90,0,0])
// manda quelli del retro (x=larghezza, y=altezza, z=spessore) in (X,Z,-Y).
color(cab_col, 0.18) {
    translate([0, cab_d/2, cab_h]) panel_extrude(panel_mat_t) top_panel();
    translate([0, cab_d/2, -panel_mat_t]) panel_extrude(panel_mat_t) bottom_panel();
    for (i = [0:cab_seg_n - 1])
        translate([-cab_w/2, cab_d/2, i*cab_seg_h + cab_seg_h/2])
            rotate([90, 0, 90])
                panel_extrude(panel_mat_t) side_panel_seg(i, true);
    for (i = [0:cab_seg_n - 1])
        translate([cab_w/2 - panel_mat_t, cab_d/2, i*cab_seg_h + cab_seg_h/2])
            rotate([90, 0, 90])
                panel_extrude(panel_mat_t) side_panel_seg(i, false);
    for (i = [0:cab_seg_n - 1])
        translate([0, cab_d, i*cab_seg_h + cab_seg_h/2])
            rotate([90, 0, 0])
                panel_extrude(panel_mat_t) back_panel_seg(i);
}
