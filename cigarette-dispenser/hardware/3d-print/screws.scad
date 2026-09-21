// Vite stampata — stampa 16x (8 housing<->fianchi, 4 tramoggia<->pannello
// superiore, 4 pannello frontale<->fianchi). È il pezzo che mancava nelle
// versioni precedenti di questa revisione: le bocchette filettate femmina
// sono già negli altri pezzi (housing.scad, cabinet.scad, front_panel.scad
// le tagliano con printed_screw_hole/screw_boss), ma la vite maschio vera e
// propria — quella che va infilata nei fori di passaggio e avvitata — non
// era mai stata istanziata come pezzo a sé stampabile.
//
// Lunghezza: 9mm va bene per tutti e tre i punti di fissaggio (verificato
// sulla profondità reale di ciascuno, vedi params.scad/i commenti nei
// rispettivi file):
//  - housing<->fianchi: il foro nel pannello è passante (9mm pannello -
//    3mm svasatura testa = 6mm di gioco), poi la vite entra ~3mm
//    nell'orecchietta filettata (spessore reale 4mm, foro tagliato un po'
//    più profondo per pulizia) — ingaggio più corto ma il carico qui è
//    basso (tiene solo l'housing, non il motore)
//  - tramoggia<->pannello superiore: foro filettato passante nel pannello
//    (9mm), la vite ingaggia quasi tutto lo spessore
//  - pannello frontale<->fianchi: foro cieco nel bordo del fianco, 8.5mm
//    di profondità — una vite da 9mm non tocca il fondo (c'è ~0.5mm di
//    gioco), la testa si appoggia comunque a filo nella svasatura
//
// Testa esagonale da 14mm: si stringe anche solo con le dita, niente
// bisogno di una chiave.
include <params.scad>
include <helpers.scad>

screw_len = 9;

module printed_screw_labeled() {
    difference() {
        printed_screw(screw_len);
        // Lettera "Q" (vedi assembly-guide.md): incisa sulla faccia esterna
        // della testa esagonale (z = -head_h, il pezzo è pieno per z
        // maggiori — stessa direzione "invertita" di coupler.scad, non si
        // può riusare label_cut così com'è)
        translate([0, 0, -printed_screw_head_h - 0.01])
            linear_extrude(height = 0.7)
                text("Q", size = 4, halign = "center", valign = "center",
                     font = "Liberation Sans:style=Bold");
    }
}

printed_screw_labeled();
