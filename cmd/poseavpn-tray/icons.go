package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"math"
)

// buildIcon draws a shield badge and wraps the PNG in an ICO container
// (Windows Vista+ accepts PNG-compressed ICO entries).
func buildIcon(body color.RGBA, ring color.RGBA, check bool) []byte {
	const size = 32
	img := image.NewRGBA(image.Rect(0, 0, size, size))

	inShield := func(x, y float64) bool {
		// shield: flat top, curved sides narrowing to a point at the bottom
		if y < 3 || y > 29 {
			return false
		}
		t := (y - 3) / 26.0 // 0 top .. 1 bottom
		half := 12.0 * (1 - math.Pow(t, 2.2))
		return math.Abs(x-15.5) <= half+0.5
	}

	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			fx, fy := float64(x)+0.5, float64(y)+0.5
			if !inShield(fx, fy) {
				continue
			}
			// ring = shield pixels whose neighbour (3px inward) is outside
			edge := !inShield(fx-2.2, fy) || !inShield(fx+2.2, fy) || !inShield(fx, fy-2.2) || !inShield(fx, fy+2.2)
			if edge {
				img.SetRGBA(x, y, ring)
			} else {
				img.SetRGBA(x, y, body)
			}
		}
	}

	if check {
		white := color.RGBA{255, 255, 255, 255}
		line := func(x0, y0, x1, y1 float64) {
			steps := 40
			for i := 0; i <= steps; i++ {
				t := float64(i) / float64(steps)
				cx, cy := x0+(x1-x0)*t, y0+(y1-y0)*t
				for dy := -1; dy <= 1; dy++ {
					for dx := -1; dx <= 1; dx++ {
						px, py := int(cx)+dx, int(cy)+dy
						if px >= 0 && px < size && py >= 0 && py < size && dx*dx+dy*dy <= 1 {
							img.SetRGBA(px, py, white)
						}
					}
				}
			}
		}
		line(10, 15, 14, 19.5)
		line(14, 19.5, 22, 10.5)
	}

	var pngBuf bytes.Buffer
	_ = png.Encode(&pngBuf, img)

	var ico bytes.Buffer
	_ = binary.Write(&ico, binary.LittleEndian, struct {
		Reserved, Type, Count uint16
	}{0, 1, 1})
	_ = binary.Write(&ico, binary.LittleEndian, struct {
		W, H, Colors, Reserved uint8
		Planes, BitCount       uint16
		Size, Offset           uint32
	}{size, size, 0, 0, 1, 32, uint32(pngBuf.Len()), 22})
	ico.Write(pngBuf.Bytes())
	return ico.Bytes()
}

var (
	iconOff  = buildIcon(color.RGBA{86, 94, 122, 255}, color.RGBA{150, 158, 184, 255}, false)
	iconOn   = buildIcon(color.RGBA{16, 185, 129, 255}, color.RGBA{110, 231, 183, 255}, true)
	iconWarn = buildIcon(color.RGBA{217, 119, 6, 255}, color.RGBA{251, 191, 36, 255}, false)
)
