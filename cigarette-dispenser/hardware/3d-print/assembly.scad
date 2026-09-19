// Visual fit-check only — NOT meant to be printed as one piece.
// Open this file in the OpenSCAD GUI to see how hub + wedges + base + lid
// go together before printing. Each part still gets printed from its own
// .scad file (hub.scad, wedge.scad x N, base.scad, lid.scad).
include <params.scad>
include <helpers.scad>
use <hub.scad>
use <wedge.scad>
use <base.scad>
use <lid.scad>

base();
hub();

for (i = [0 : slots - 1])
    rotate([0, 0, i * 360 / slots])
        wedge();

color("lightblue", 0.4)
    translate([0, 0, hub_height])
        lid();
