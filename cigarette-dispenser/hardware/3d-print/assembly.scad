// Anteprima visiva SOLO per orientarsi — non da stampare. Apri in OpenSCAD
// per farti un'idea di come stanno insieme rullo/housing/tramoggia/scivolo/
// pannello. Il collegamento esatto scivolo->pannello dipende dalla
// profondità del tuo mobile: adattalo in fase di assemblaggio reale.
include <params.scad>
use <roller.scad>
use <housing.scad>
use <hopper.scad>
use <chute.scad>
use <front_panel.scad>

// roller.scad/housing.scad sono modellati con l'asse di rotazione lungo Z;
// li ruotiamo di -90° su X per farli giacere orizzontali (asse lungo Y).
rotate([-90, 0, 0]) {
    color("orange") roller();
    color("lightgray", 0.35) housing();
}

// tramoggia sopra l'apertura di carico (in alto, lato +Z prima della
// rotazione -> +Y dopo)
translate([0, housing_or + housing_clear, 0])
    rotate([90, 0, 0])
        color("yellow", 0.5) hopper();

// scivolo sotto l'apertura di scarico (lato -Y nel frame ruotato)
translate([0, -housing_or - 40, roller_len/2])
    rotate([90, 0, 0])
        color("lightblue", 0.6) chute();

// pannello frontale, oltre lo scivolo
translate([-80, -housing_or - 100, roller_len/2 - 100])
    color("white") front_panel();
