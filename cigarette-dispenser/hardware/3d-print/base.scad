// Stationary base plate — print 1x. The carousel (hub + wedges) sits and
// spins on top of this. Every pocket's floor IS this plate, except at the
// chute cutout: that's the only spot a pack can fall through, and it only
// lines up with one pocket at a time as the stepper indexes the carousel.
include <params.scad>
include <helpers.scad>

plate_od      = r_out * 2 + 10;   // overall plate diameter
plate_t       = wall * 2;         // plate thickness
bearing_od    = 22;   // 608-size skate bearing OD (swap for your bearing)
bearing_boss_h= 6;

// NEMA17-ish mounting pattern (also fits most small gearmotors if you
// re-drill): 31mm bolt spacing, M3 clearance, adjust to your motor's specs.
motor_hole_spacing = 31;
motor_hole_d        = 3.4;
motor_bore_d         = 24;  // clearance for the motor's boss/shaft collar

// chute cutout: same sector as one pocket, oversized slightly so it clears
// the pocket that is dropping, positioned at angle 0 (see wiring.md for how
// the home/index sensor is used to align pocket 0 here at startup).
chute_margin = clear;

module base() {
    difference() {
        union() {
            cylinder(d = plate_od, h = plate_t, $fn = 128);
            // bearing boss under the hub, on the underside
            translate([0, 0, -bearing_boss_h])
                cylinder(d = bearing_od + wall*2, h = bearing_boss_h, $fn = 64);
        }
        // bearing seat (press-fit a 608 skate bearing, or substitute a
        // plain PTFE-lined bushing sized to your motor shaft coupler)
        translate([0, 0, -bearing_boss_h - 1])
            cylinder(d = bearing_od, h = bearing_boss_h + 2, $fn = 64);
        // motor shaft clearance through the plate
        translate([0, 0, -1])
            cylinder(d = motor_bore_d, h = plate_t + bearing_boss_h + 2, $fn = 48);
        // motor mounting holes
        for (i = [0:3])
            rotate([0, 0, i*90 + 45])
                translate([motor_hole_spacing/sqrt(2), 0, -bearing_boss_h - 1])
                    cylinder(d = motor_hole_d, h = bearing_boss_h + plate_t + 2, $fn = 16);
        // the chute: same sector shape as a pocket, at angle 0, oversized
        // by chute_margin so it fully clears whichever pocket sits over it
        translate([0, 0, -1])
            linear_extrude(height = plate_t + 2)
                ring_slice(r_in - chute_margin, r_out + chute_margin, ang + 2*chute_margin);
    }
}

base();
