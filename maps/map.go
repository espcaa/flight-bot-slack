package maps

import (
	structs "flight-tracker-slack/types"
	"fmt"
	"image"
	"image/color"
	"os"

	sm "github.com/flopp/go-staticmaps"
	"github.com/fogleman/gg"
	"github.com/golang/geo/s2"
	"github.com/google/uuid"
)

func GenerateAircraftMap(lat float64, lon float64, tracks []structs.TrackPoint, heading int) (string, error) {
	ctx := sm.NewContext()
	ctx.SetSize(1200, 900)
	ctx.SetZoom(10)

	aircraftPos := s2.LatLngFromDegrees(lat, lon)

	osfile, err := os.Open("assets/plane.png")
	if err != nil {
		return "", fmt.Errorf("failed to open plane image: %w", err)
	}
	defer osfile.Close()

	imga, _, err := image.Decode(osfile)
	if err != nil {
		return "", fmt.Errorf("failed to decode plane image: %w", err)
	}

	dc := gg.NewContextForImage(imga)
	dc.RotateAbout(gg.Radians(float64(heading)), float64(imga.Bounds().Dx()/2), float64(imga.Bounds().Dy()/2))
	rotated := dc.Image()
	ctx.OverrideAttribution("")
	ctx.AddObject(sm.NewImageMarker(aircraftPos, rotated, 25/2, 25/2))

	if len(tracks) > 1 {
		pathPositions := make([]s2.LatLng, 0, len(tracks))
		for _, t := range tracks {
			pathPositions = append(pathPositions, s2.LatLngFromDegrees(t.Coord[1], t.Coord[0]))
		}
		ctx.AddObject(sm.NewPath(pathPositions, color.RGBA{221, 55, 255, 255}, 3.0))
	}

	img, err := ctx.Render()
	if err != nil {
		return "", fmt.Errorf("failed to render map: %w", err)
	}

	fileName := fmt.Sprintf("aircraft-map-%s.png", uuid.New().String())
	if err := gg.SavePNG(fileName, img); err != nil {
		return "", fmt.Errorf("failed to save map PNG: %w", err)
	}

	return fileName, nil
}
