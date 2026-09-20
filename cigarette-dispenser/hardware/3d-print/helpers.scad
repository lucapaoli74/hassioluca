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

// --- Identificazione pezzi: una lettera incisa su ogni pezzo stampato ------
// Vedi la legenda completa (A-O) in assembly-guide.md. Uso: mettere questa
// come ultima operazione di un difference() sul solido del pezzo, dopo tutti
// gli altri fori/tagli. `ztop` è la quota della faccia piatta su cui incidere
// (nel sistema di coordinate locale del pezzo in quel punto); il taglio parte
// 0.8mm sotto e arriva 0.5mm sopra la faccia, apposta più profondo della sola
// incisione per evitare superfici coincidenti (stesso problema di mesh non
// manifold già visto altrove in questo progetto se il taglio si fermasse
// esattamente sulla superficie).
module label_cut(letter, x, y, ztop, depth = 0.8, size = 7) {
    translate([x, y, ztop - depth])
        linear_extrude(height = depth + 0.5)
            text(letter, size = size, halign = "center", valign = "center",
                 font = "Liberation Sans:style=Bold");
}

// Come label_cut, ma per una faccia con normale lungo +Y invece di +Z (il
// caso di front_panel.scad, dove lo spessore del pannello è lungo Y, non
// Z). Stessa tecnica rotate([90,0,0]) verificata per label_tag: senza
// questa versione, label_cut inciderebbe nella direzione sbagliata.
module label_cut_y(letter, x, yface, z, depth = 0.8, size = 7) {
    translate([x, yface + 0.5, z])
        rotate([90, 0, 0])
            linear_extrude(height = depth + 0.5)
                text(letter, size = size, halign = "center", valign = "center",
                     font = "Liberation Sans:style=Bold");
}

// Targhetta autoportante per pezzi troppo sottili per un'incisione diretta
// (griglia, scivolo): un piccolo blocco pieno fuso per unione con un margine
// di sovrapposizione reale nel pezzo esistente (non solo a contatto), con la
// lettera incisa sulla faccia esterna. `w`/`d`/`h` sono le dimensioni del
// blocco lungo X/Y/Z; il chiamante lo posiziona/ruota così che l'estremità
// -Y penetri nel pezzo di `overlap` mm.
module label_tag(letter, w = 12, d = 6, h = 6, overlap = 2.5, depth = 0.7, size = 5.5) {
    difference() {
        translate([-w/2, -overlap, -h/2])
            cube([w, d + overlap, h]);
        // rotate([90,0,0]) manda l'estrusione di testo (di default lungo +Z,
        // faccia a z=0) a un'estrusione lungo -Y con la faccia a y=0 —
        // verificato esportando solo questo taglio e leggendo il bounding box
        // risultante, non a occhio. Traslando quella faccia a y=d+0.5 il
        // taglio arriva da y=d-depth (dentro il blocco) a y=d+0.5 (oltre la
        // faccia esterna a y=d).
        translate([0, d + 0.5, 0])
            rotate([90, 0, 0])
                linear_extrude(height = depth + 0.5)
                    text(letter, size = size, halign = "center", valign = "center",
                         font = "Liberation Sans:style=Bold");
    }
}
