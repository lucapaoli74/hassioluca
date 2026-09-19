// Rotary hub — print 1x, PETG or ABS recommended (PLA wears fast at the bore).
// Carries the motor shaft coupler and gives each wedge (wedge.scad) a screw
// boss to bolt onto.
include <params.scad>

module hub() {
    difference() {
        union() {
            cylinder(d = hub_od, h = hub_height, $fn = 96);
            for (i = [0 : slots - 1])
                rotate([0, 0, i * 360 / slots])
                    translate([hub_od/2 - boss_d/2 - 1, 0, 0])
                        cylinder(d = boss_d, h = hub_height, $fn = 24);
        }
        // motor shaft coupler bore
        translate([0, 0, -1])
            cylinder(d = hub_id, h = hub_height + 2, $fn = 48);
        // grub-screw access hole for the shaft coupler
        translate([-hub_od/2 - 1, 0, hub_height/2])
            rotate([0, 90, 0])
                cylinder(d = 3.2, h = hub_od + 2, $fn = 24);
        // screw holes into the attachment bosses
        for (i = [0 : slots - 1])
            rotate([0, 0, i * 360 / slots])
                translate([hub_od/2 - boss_d/2 - 1, 0, -1])
                    cylinder(d = boss_hole, h = hub_height + 2, $fn = 24);
    }
}

hub();
