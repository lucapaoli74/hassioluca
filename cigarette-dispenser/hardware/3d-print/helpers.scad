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

// --- Viti stampate: filettatura grossa "propria" (non metrica) --------------
// Non imita una vite M3/M4 vera — un passo/profondo dente pensati apposta per
// l'FDM (una M3 vera, passo 0.5mm, quasi sempre non tiene stampata). Vite e
// bocchetta vengono generate dalla STESSA funzione (thread_ridge), quindi
// combaciano sempre tra loro anche se cambi i parametri in params.scad — non
// devono combaciare con niente comprato.
//
// Tecnica: un dente triangolare spazzato con linear_extrude(twist=...) attorno
// a un nucleo cilindrico. Verificato in sessione che serve un margine di
// sovrapposizione (`overlap`) tra la base del dente e il nucleo, altrimenti le
// due superfici sono tangenti e la mesh non è manifold (stesso problema già
// visto altrove in questo progetto) — controllato esportando vite+bocchetta
// da sole e leggendo "Simple: yes" nell'output di OpenSCAD, non a occhio.
module thread_ridge(major_d, pitch, length, tooth_h, overlap = 0.3) {
    turns = length / pitch;
    r_apex = major_d / 2;
    r_base = r_apex - tooth_h - overlap;
    linear_extrude(height = length, twist = 360 * turns,
                    slices = max(24, turns * 32), convexity = 10)
        translate([r_base, 0])
            polygon(points = [[0, -pitch * 0.32], [tooth_h + overlap, 0], [0, pitch * 0.32]]);
}

// Vite maschio: nucleo + filetto, con una testa esagonale (facile da stampare
// senza supporti, si stringe anche solo con le dita data la coarse pitch).
// L'asse è Z: la testa sta a z<0, il filetto da z=0 a z=length (entra nella
// bocchetta lungo +Z).
module printed_screw(length, d = printed_screw_d, pitch = printed_screw_pitch,
                      tooth_h = printed_screw_tooth_h,
                      head_d = printed_screw_head_d, head_h = printed_screw_head_h) {
    union() {
        translate([0, 0, -head_h])
            cylinder(d = head_d, h = head_h, $fn = 6);
        cylinder(d = d - 2 * tooth_h, h = length, $fn = 40);
        thread_ridge(d, pitch, length, tooth_h);
    }
}

// Foro filettato femmina: da sottrarre da una bocchetta piena (screw_boss
// sotto, o direttamente dal materiale del pezzo se è abbastanza spesso).
// Stesso filetto della vite, allargato di `clearance` per il gioco di stampa
// — se il tuo filamento/stampante stringe troppo (o gira a vuoto), è questo
// il valore da regolare per primo.
module printed_screw_hole(length, d = printed_screw_d, pitch = printed_screw_pitch,
                           tooth_h = printed_screw_tooth_h, clearance = printed_screw_clearance) {
    union() {
        cylinder(d = d + 2 * clearance, h = length, $fn = 40);
        thread_ridge(d + 2 * clearance, pitch, length, tooth_h);
    }
}

// Bocchetta piena (da unire al pezzo) con dentro il foro filettato già
// sottratto — un solo modulo per "aggiungi materiale qui e filettalo".
// `od` è il diametro esterno della bocchetta (default: abbastanza materiale
// pieno intorno al filetto da non spaccarsi stringendo).
module screw_boss(length, od = printed_screw_d + 6, d = printed_screw_d,
                   pitch = printed_screw_pitch, tooth_h = printed_screw_tooth_h,
                   clearance = printed_screw_clearance) {
    difference() {
        cylinder(d = od, h = length, $fn = 48);
        translate([0, 0, -0.5])
            printed_screw_hole(length + 1, d, pitch, tooth_h, clearance);
    }
}

// --- Spine di centraggio stampate (incastro + colla) -------------------------
// Sostituiscono gli angolari metallici agli spigoli del mobile: una spina
// cilindrica su un pezzo, un foro cieco corrispondente sull'altro. Non sono
// pensate per reggere da sole — allineano i pezzi durante l'incollaggio e
// danno un po' di resistenza a taglio, il grosso della tenuta è la colla
// sulla superficie di contatto tra i due pannelli.
module dowel_peg(d = dowel_d, h = dowel_h) {
    cylinder(d = d, h = h, $fn = 24);
}

module dowel_socket(d = dowel_d, h = dowel_h, clearance = dowel_clearance) {
    cylinder(d = d + 2 * clearance, h = h + 0.5, $fn = 24);
}

// --- Cerniera stampata (nocche + perno in filamento) ------------------------
// Una nocca piena (da unire al pezzo): asse lungo X (rotate([0,90,0]) manda
// l'asse nativo Z di cylinder() lungo X — stessa tecnica verificata per
// label_tag/i fori di taglio nei fianchi). `y`/`z` sono la quota della
// cerniera nel sistema di coordinate LOCALE del pezzo che la chiama: sta al
// chiamante calcolare la quota giusta perché coincida con l'altro pezzo una
// volta assemblati (vedi il conto in hopper.scad/hopper_lid.scad).
module hinge_knuckle_solid(x, y, z, w = hinge_knuckle_print_w) {
    translate([x, y, z])
        rotate([0, 90, 0])
            cylinder(d = hinge_knuckle_od, h = w, center = true, $fn = 24);
}

// Foro perno da sottrarre — stessa retta su entrambi i pezzi per
// costruzione se gli passi lo stesso (y,z), non due fori indipendenti da
// allineare a mano.
module hinge_pin_cut(y, z, span = 60) {
    translate([-span/2, y, z])
        rotate([0, 90, 0])
            cylinder(d = hinge_pin_d, h = span, $fn = 16);
}

// --- Giunto a pettine 2D (giunzione tra segmenti spezzati) ------------------
// Pettine di denti larghi `fw` lungo un segmento di lunghezza `length`,
// presenti solo dove l'indice del dente ha la parità `phase` (0 o 1) — usato
// a coppie: un pannello aggiunge alla fase 0 e taglia alla fase 1, il
// pannello che ci si incastra fa il contrario, così i denti di uno riempiono
// esattamente i vuoti dell'altro. Il numero di denti è arrotondato perché
// `length` sia coperta esattamente (nessun resto storto a un'estremità).
module finger_teeth(length, fw, depth, phase) {
    n = round(length / fw);
    afw = length / n;
    for (i = [0 : n - 1])
        if ((i % 2) == phase)
            translate([i * afw, 0])
                square([afw, depth]);
}
