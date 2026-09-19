// Top lid — print 1x. Covers the carousel so only one pocket at a time is
// reachable, through a fixed loading port with a sliding door. Load packs
// by cycling the carousel (dispense a slot, or use the "load" button in
// Home Assistant, see software/home-assistant) until an empty pocket sits
// under the port, then slide the door open and drop a pack in.
include <params.scad>
include <helpers.scad>

lid_od     = r_out * 2 + 10;   // match base.scad's plate_od
lid_t      = wall;
rim_h      = 8;                // skirt that keeps the lid centered on the base
port_r1    = r_in - clear;
port_r2    = r_out + clear;
door_t     = wall;
rail_gap   = 0.6;              // printed-in-place sliding fit clearance

module lid() {
    difference() {
        union() {
            cylinder(d = lid_od, h = lid_t, $fn = 128);
            // skirt
            translate([0, 0, -rim_h])
                difference() {
                    cylinder(d = lid_od, h = rim_h, $fn = 128);
                    translate([0, 0, -1])
                        cylinder(d = lid_od - wall*4, h = rim_h + 2, $fn = 128);
                }
            // door rails either side of the loading port (angle 0)
            for (s = [-1, 1])
                rotate([0, 0, s * (ang/2 + 2)])
                    translate([0, 0, lid_t])
                        linear_extrude(height = 4)
                            ring_slice(port_r1 - 3, port_r2 + 3, 6);
        }
        // loading port: same sector as one pocket
        translate([0, 0, -1])
            linear_extrude(height = lid_t + 2)
                ring_slice(port_r1, port_r2, ang);
        // center bore, clears the hub + shaft coupler
        translate([0, 0, -rim_h - 1])
            cylinder(d = hub_od + 4, h = rim_h + lid_t + 2, $fn = 96);
    }
}

// Sliding door for the loading port — print separately, slides in the rails.
module door() {
    linear_extrude(height = door_t)
        ring_slice(port_r1 - 3 + rail_gap, port_r2 + 3 - rail_gap, ang + 2*(2 - rail_gap));
}

lid();
translate([lid_od/2 + 20, 0, 0]) door();
