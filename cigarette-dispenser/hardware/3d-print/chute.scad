// Scivolo di scarico — stampa 1x. Si fissa sotto l'apertura di scarico
// dell'housing e incanala la sigaretta singola verso il pannello frontale,
// spostandosi in orizzontale di `chute_dy` (params.scad) per raggiungerlo:
// il mobile mette la tramoggia/housing al centro e il pannello vicino al
// fronte, quindi lo scivolo è un condotto obliquo, non dritto.
include <params.scad>
include <helpers.scad>

module loft(w1, d1, w2, d2, h, dy2 = 0) {
    hull() {
        translate([0, 0, h]) linear_extrude(0.01) square([w1, d1], center = true);
        translate([0, -dy2, 0]) linear_extrude(0.01) square([w2, d2], center = true);
    }
}

id_top    = max(drop_d1 - 2*hopper_wall, 4);
id_bottom = max(drop_d2 - 2*hopper_wall, 4);
iw_top    = drop_w - 2*hopper_wall;
iw_bottom = drop_w - 6 - 2*hopper_wall;

function lerp(a, b, t) = a + (b - a) * t;

// Sbarramento anti-intrusione: due alette sfalsate (una da un lato, una
// dall'altro) impediscono a una mano infilata dalla feritoia del pannello
// di raggiungere in linea retta il rullo, sullo stesso principio di una
// buca delle lettere o di una cassaforte con imbuto a zig-zag. La
// sigaretta (piccola, cade tumbling) passa lo stesso dallo spazio lasciato
// libero sul lato opposto.
// centro (in Y) della sezione del condotto all'altezza z0 — il condotto è
// obliquo (vedi loft/dy2), quindi non è fisso a Y=0 come prima
function center_y(z0) = lerp(-chute_dy, 0, z0 / drop_h);

module baffle(z0, side, cover_frac, thick, overlap = 2) {
    t = z0 / drop_h;
    d = lerp(id_bottom, id_top, t);
    w = lerp(iw_bottom, iw_top, t) - 1; // piccolo gioco dalle pareti
    cov = d * cover_frac;
    // il bordo esterno affonda `overlap` nella parete (unione solida,
    // non tangente) — lo stesso accorgimento usato altrove per evitare
    // mesh non manifold da superfici coincidenti
    y_far  = center_y(z0) + side * (d/2 + overlap);
    y_near = center_y(z0) + side * (d/2 - cov);
    translate([0, (y_far + y_near) / 2, z0])
        cube([w, abs(y_far - y_near), thick], center = true);
}

// Lettera "F" (vedi assembly-guide.md): la parete è troppo sottile
// (hopper_wall) per un'incisione diretta, quindi una piccola targhetta
// sporge dalla parete esterna lato +Y (verso il fronte), vicino alla cima —
// posizione calcolata dalle stesse funzioni lerp/center_y del condotto
// (non a occhio), con `overlap` scelto apposta perché resti dentro lo
// spessore reale della parete a quella quota, senza sporgere nella cavità.
module f_tag() {
    z0 = drop_h * 0.85;
    t  = z0 / drop_h;
    outer_w = lerp(drop_w - 6, drop_w, t);
    outer_d = lerp(drop_d2, drop_d1, t);
    y_face  = center_y(z0) + outer_d/2;
    translate([0, y_face, z0])
        label_tag("F", w = 12, d = 5, h = 6, overlap = 2, depth = 0.7, size = 5);
}

module chute() {
    union() {
        difference() {
            loft(drop_w, drop_d1, drop_w - 6, drop_d2, drop_h, chute_dy);
            translate([0, 0, -1])
                loft(iw_top, id_top, iw_bottom, id_bottom, drop_h + 2, chute_dy);
        }
        // le alette si aggiungono DOPO aver scavato la cavità, altrimenti
        // la sottrazione le rimuoverebbe (sono dimensionate per stare
        // proprio dentro quello spazio cavo)
        baffle(drop_h * 0.66, 1, 0.62, 6);
        baffle(drop_h * 0.33, -1, 0.62, 6);
        f_tag();
    }
}

chute();
