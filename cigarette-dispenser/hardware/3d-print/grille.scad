// Griglia di sicurezza — stampa 1x. Va incastrata tra la bocca inferiore
// della tramoggia (hopper.scad) e l'apertura di carico dell'alloggiamento
// (housing.scad): è l'unica cosa che un dito incontra guardando dentro la
// tramoggia, molto più vicina della scanalatura in rotazione.
//
// NON è una certificazione di sicurezza — è un accorgimento ragionevole
// per un progetto domestico: le celle sono più strette del calibro da
// 12mm usato nelle norme macchine (EN ISO 13857) per verificare che un
// dito non passi, e restano comunque più larghe del diametro sigaretta
// per farla cadere. Se costruisci per un contesto con bambini piccoli,
// valuta celle ancora più strette (riduci `cell_gap`) verificando che le
// sigarette continuino a passare con la tramoggia che vibra.
include <params.scad>

cell_gap  = 11;    // luce libera tra le barre (< probe da 12mm, > cig_d)
bar_w     = 3;      // spessore barra
bar_h     = 6;       // altezza barra (irrigidisce la griglia)
plate_t   = 2.5;     // spessore del bordo/telaio
lip       = 3;        // sporgenza del bordo oltre l'apertura, per l'appoggio

frame_w = hopper_bot_w + 2*lip;
frame_d = hopper_bot_d + 2*lip;

module bars(len_x, len_y, span_x, span_y) {
    pitch = cell_gap + bar_w;
    nx = floor(span_x / pitch);
    ny = floor(span_y / pitch);
    // barre lungo Y (spaziate su X)
    for (i = [0:nx])
        translate([-span_x/2 + i*pitch, 0, 0])
            cube([bar_w, len_y, bar_h], center = true);
    // barre lungo X (spaziate su Y)
    for (j = [0:ny])
        translate([0, -span_y/2 + j*pitch, 0])
            cube([len_x, bar_w, bar_h], center = true);
}

module grille() {
    union() {
        // telaio di appoggio (bordo pieno che si posa tra tramoggia e housing)
        difference() {
            cube([frame_w, frame_d, plate_t], center = true);
            cube([hopper_bot_w, hopper_bot_d, plate_t + 2], center = true);
        }
        // griglia a maglia, centrata sull'apertura utile
        translate([0, 0, plate_t/2 + bar_h/2 - 0.01])
            bars(hopper_bot_w, hopper_bot_d, hopper_bot_w - bar_w, hopper_bot_d - bar_w);
    }
}

grille();
