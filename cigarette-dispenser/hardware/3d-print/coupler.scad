// Accoppiatore stampato albero motore <-> rullo — stampa 1x. Sostituisce
// il giunto flessibile comprato: lato motore un foro con un grano
// (vite in miniatura stampata, helpers.scad) che preme sull'alberino;
// lato rullo uno stelo che entra nel foro albero del rullo (shaft_d,
// roller.scad) e viene bloccato dalla stessa spina trasversale che il
// rullo aveva già (roller_pin_z in params.scad) — uno spezzone di filo/
// chiodo da 3mm, o una spina stampata a pressione, non serve altro.
//
// ATTENZIONE — è il pezzo più sperimentale di questa revisione: un grano
// premuto su un albero liscio è una tecnica comune per pulegge/ingranaggi
// stampati (attrito/carico bassi come qui), ma non è stato possibile
// stampare e provare questo pezzo sotto carico reale in questa sessione.
// Collauda a vuoto (vedi il passo 12 in 3d-print/README.md) prima di
// fidartene per l'uso quotidiano; se noti slittamento, la soluzione di
// riserva resta il giunto flessibile comprato in bom.md.
//
// MISURA IL TUO MOTORE: motor_shaft_d in params.scad assume 5mm (tipico
// NEMA17) — controlla il datasheet del tuo motore prima di stampare.
include <params.scad>
include <helpers.scad>

motor_bore_d = motor_shaft_d + coupler_motor_clear;
roller_stub_d = shaft_d - 2*coupler_roller_clear;
total_len = coupler_motor_len + coupler_septum_t + coupler_roller_len;

module coupler() {
    difference() {
        union() {
            // corpo principale: contiene il foro lato motore + il setto pieno
            cylinder(d = coupler_od, h = coupler_motor_len + coupler_septum_t, $fn = 48);
            // stelo lato rullo, più stretto, sporge dall'altra estremità
            translate([0, 0, coupler_motor_len + coupler_septum_t])
                cylinder(d = roller_stub_d, h = coupler_roller_len, $fn = 32);
        }
        // foro cieco lato motore
        translate([0, 0, -0.5])
            cylinder(d = motor_bore_d, h = coupler_motor_len + 0.5, $fn = 32);
        // grano di bloccaggio: preme radialmente sull'alberino motore, a
        // metà della profondità del foro — rotate([90,0,0]) manda
        // l'estrusione nativa di printed_screw_hole (lungo +Z) lungo -Y,
        // quindi partendo da fuori il pezzo (y = od/2+0.5) e con lunghezza
        // od/2+0.5 arriva esattamente al centro (stessa tecnica verificata
        // per i fori di taglio nei fianchi/pannello frontale).
        translate([0, coupler_od/2 + 0.5, coupler_motor_len/2])
            rotate([90, 0, 0])
                printed_screw_hole(coupler_od/2 + 0.5, d = grub_d, pitch = grub_pitch,
                                    tooth_h = grub_tooth_h, clearance = grub_clearance);
        // foro per la spina di bloccaggio lato rullo — alla stessa quota
        // (roller_pin_z, misurata dall'inizio dello stelo) del foro
        // trasversale già presente in roller.scad, così i due combaciano
        // una volta infilato lo stelo nel rullo
        translate([-coupler_od/2 - 1, 0, coupler_motor_len + coupler_septum_t + roller_pin_z])
            rotate([0, 90, 0])
                cylinder(d = coupler_pin_d, h = coupler_od + 2, $fn = 16);
        // Lettera "P" (vedi assembly-guide.md): incisa sulla faccia
        // accessibile lato motore (z=0, il pezzo è pieno per z>0 —
        // direzione opposta a label_cut, che assume materiale sotto ztop
        // non sopra, quindi qui il taglio va scritto a mano)
        translate([5, 0, -0.01])
            linear_extrude(height = 1.3)
                text("P", size = 3.5, halign = "center", valign = "center",
                     font = "Liberation Sans:style=Bold");
    }
}

coupler();
