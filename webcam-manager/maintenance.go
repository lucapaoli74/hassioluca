package main

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"

	xdraw "golang.org/x/image/draw"
)

const defaultMaintenanceText = "Webcam in manutenzione"

func (s *Scheduler) rawPath(id string) string {
	return filepath.Join(s.dataDir, "latest", id+".raw.jpg")
}

func (s *Scheduler) maintenancePath(id string) string {
	return filepath.Join(s.dataDir, "maintenance", id+".jpg")
}

// FreezeMaintenanceImage memorizza l'ultima immagine buona della webcam come
// sfondo da mostrare durante la manutenzione.
func (s *Scheduler) FreezeMaintenanceImage(id string) {
	img := s.Raw(id)
	if img == nil {
		img, _ = os.ReadFile(s.rawPath(id))
	}
	if img == nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(s.maintenancePath(id)), 0o755)
	_ = writeAtomic(s.maintenancePath(id), img)
}

// maintenanceBackground: immagine congelata, altrimenti l'ultima disponibile,
// altrimenti un fondo neutro.
func (s *Scheduler) maintenanceBackground(id string) []byte {
	for _, p := range []string{s.maintenancePath(id), s.rawPath(id)} {
		if b, err := os.ReadFile(p); err == nil {
			return b
		}
	}
	if b := s.Raw(id); b != nil {
		return b
	}
	return TestPattern(1280, 720)
}

// maintenanceCamera restituisce la webcam con l'avviso aggiunto in primo
// piano e senza storico/timelapse (non si archiviano immagini ferme).
func maintenanceCamera(cam Camera) Camera {
	text := cam.MaintenanceText
	if text == "" {
		text = defaultMaintenanceText
	}
	m := cam
	m.Overlays = append(append([]Overlay(nil), cam.Overlays...), Overlay{
		Type: "text", Enabled: true, Text: text, Anchor: "center", Size: 7,
		Color: "#ffffff", Bold: true, Shadow: true, Background: "#9a3412", BgOpacity: 0.85, Opacity: 1,
	})
	m.ArchiveMinutes = 0
	m.TimelapseMinutes = 0
	return m
}

// darkenJPEG scurisce l'immagine per far capire che non è in diretta.
func darkenJPEG(src []byte) ([]byte, error) {
	img, err := jpeg.Decode(bytes.NewReader(src))
	if err != nil {
		return nil, err
	}
	dst := image.NewRGBA(img.Bounds())
	xdraw.Draw(dst, dst.Bounds(), img, img.Bounds().Min, xdraw.Src)
	xdraw.Draw(dst, dst.Bounds(), image.NewUniform(color.NRGBA{0, 0, 0, 120}), image.Point{}, xdraw.Over)
	var out bytes.Buffer
	if err := jpeg.Encode(&out, dst, &jpeg.Options{Quality: 88}); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
