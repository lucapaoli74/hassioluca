// Pocket wedge — print `slots` copies (see params.scad), one per pocket.
// Each wedge is an open-top, open-bottom tube: the STATIONARY base plate
// (base.scad) is what actually holds the packs up. Only where the base has
// its chute cutout does a pocket's bottom open up, so a pack drops out
// exactly when the carousel rotates that pocket over the chute.
include <params.scad>
include <helpers.scad>

tab_r = hub_od/2 - boss_d/2 - 1;  // must match the boss position in hub.scad

// Screw tab, connected to the wedge's inner wall by a hull() bridge so it's
// always one solid part regardless of the gap between tab_r and r_in.
module tab() {
    difference() {
        hull() {
            translate([tab_r, 0, 0])
                cylinder(d = boss_d + wall*2, h = hub_height, $fn = 24);
            // reach past r_in into the ring's solid wall (r_in .. r_in+wall)
            // so the hull actually overlaps solid material, not the hollow
            // pocket interior — a 1mm-short bridge left a manifold gap here.
            translate([r_in + wall + 1, 0, 0])
                cylinder(d = boss_d + wall*2, h = hub_height, $fn = 24);
        }
        translate([tab_r, 0, -1])
            cylinder(d = boss_hole, h = hub_height + 2, $fn = 24);
    }
}

module wedge() {
    union() {
        linear_extrude(height = hub_height)
            difference() {
                ring_slice(r_in, r_out, ang);
                offset(delta = -wall) ring_slice(r_in, r_out, ang);
            }
        tab();
    }
}

wedge();
