// Shared 2D helpers used by wedge.scad, base.scad and lid.scad.

// Big half-plane (y >= 0), used to build angle masks. `big` just has to be
// larger than any radius used in the design.
module halfplane(big = 2000) {
    translate([-big/2, 0]) square([big, big]);
}

// An angular mask covering [-a/2, +a/2], built as the intersection of two
// half-planes rather than an origin-apex polygon: a sector polygon with its
// point pinned exactly at (0,0) tends to produce a self-touching ("bowtie")
// result once differenced against a same-apex sector for the inner radius,
// which extrudes into a non-manifold solid. Two rotated half-planes never
// share that degenerate vertex.
module angle_mask(a) {
    intersection() {
        rotate([0, 0, -a/2]) halfplane();
        rotate([0, 0, a/2 - 180]) halfplane();
    }
}

// A ring slice between radius r1 and r2, angular width a, centered on angle 0.
module ring_slice(r1, r2, a) {
    intersection() {
        difference() {
            circle(r = r2, $fn = 128);
            circle(r = r1, $fn = 128);
        }
        angle_mask(a);
    }
}
