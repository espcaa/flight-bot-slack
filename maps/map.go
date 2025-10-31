package maps

import (
	"fmt"
	"image/color"

	structs "flight-tracker-slack/types"

	sm "github.com/flopp/go-staticmaps"
	"github.com/fogleman/gg"
	"github.com/golang/geo/s2"
	"github.com/google/uuid"
)

func GenerateAircraftMap(lat float64, lon float64, tracks []structs.TrackPoint) (string, error) {
	ctx := sm.NewContext()
	ctx.SetSize(800, 600)
	ctx.SetZoom(0)

	aircraftPos := s2.LatLngFromDegrees(lat, lon)
	ctx.AddObject(sm.NewMarker(aircraftPos, color.RGBA{0, 255, 0, 255}, 16.0))

	if len(tracks) > 1 {
		pathPositions := make([]s2.LatLng, 0, len(tracks))
		for _, t := range tracks {
			pathPositions = append(pathPositions, s2.LatLngFromDegrees(t.Coord[0], t.Coord[1]))
		}
		ctx.AddObject(sm.NewPath(pathPositions, color.RGBA{0, 0, 255, 255}, 3.0))
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
