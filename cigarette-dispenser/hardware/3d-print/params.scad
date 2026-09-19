// Shared parameters for the whole dispenser — edit these once, every part
// (hub, wedges, base, lid) picks them up via `include`.
// Measure your own packs before printing: hard packs vary a few mm by brand.

slots      = 6;     // number of pockets around the carousel
pack_w     = 55;    // pack width (mm)  - sits tangentially (packs stand upright)
pack_d     = 22;    // pack depth (mm)  - sits radially
pack_h     = 85;    // pack height (mm) - packs stand upright in the pocket
clear      = 3;     // per-side clearance around a pack
wall       = 2.4;   // wall thickness (~6 perimeters at 0.4mm nozzle, PETG)

gap_deg    = 2;                     // angular gap left between neighboring wedges
ang        = 360/slots - gap_deg;   // usable angle per wedge

hub_margin = 6;      // extra radius beyond the strict minimum (wall + boss room)

// r_in is derived, not hand-picked: the tangential width available at r_in
// for one wedge (a chord of angle `ang`) must be >= pack_w + 2*clear, or the
// pack won't fit — sizing this by hand for hub.scad, wedge.scad and base.scad
// separately caused pockets narrower than the pack in an earlier revision.
r_in       = (pack_w + 2*clear) / (2*sin(ang/2)) + hub_margin;
r_out      = r_in + pack_d + clear;

hub_od     = 2 * (r_in - 2);  // hub sits just inside r_in
hub_id     = 8;               // bore for the motor shaft coupler (mm)
hub_height = pack_h + clear + wall;

boss_d     = 6;      // hub attachment boss diameter
boss_hole  = 3.2;    // clearance hole for M3 screw
