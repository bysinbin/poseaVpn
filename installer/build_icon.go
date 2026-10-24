//go:build ignore

package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
)

func renderIcon(size int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	s := float64(size)

	// Squircle / rounded rect background
	r := s * 0.2237 // Apple squircle radius ratio
	cx, cy := s/2, s/2

	inSquircle := func(x, y float64) bool {
		dx := math.Abs(x - cx)
		dy := math.Abs(y - cy)
		half := s/2 - s*0.04
		if dx > half || dy > half {
			return false
		}
		cornX := half - r
		cornY := half - r
		if dx > cornX && dy > cornY {
			cdx := dx - cornX
			cdy := dy - cornY
			return cdx*cdx+cdy*cdy <= r*r
		}
		return true
	}

	// Shield geometry (scaled to size)
	// center at (cx, cy + s*0.02)
	inShield := func(x, y float64) bool {
		sx := x - cx
		sy := y - (cy - s*0.02)
		shieldH := s * 0.54
		shieldW := s * 0.44

		if sy < -shieldH/2 || sy > shieldH/2 {
			return false
		}
		t := (sy + shieldH/2) / shieldH // 0 (top) .. 1 (bottom)
		halfW := (shieldW / 2) * (1.0 - math.Pow(t, 2.2))
		return math.Abs(sx) <= halfW
	}

	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			fx := float64(x) + 0.5
			fy := float64(y) + 0.5

			if !inSquircle(fx, fy) {
				img.SetRGBA(x, y, color.RGBA{0, 0, 0, 0})
				continue
			}

			// Dark gradient background (PoseaVPN dark theme)
			tGrad := (fx + fy) / (s * 2)
			bgR := uint8(10 + tGrad*12)
			bgG := uint8(14 + tGrad*18)
			bgB := uint8(26 + tGrad*30)

			// Shield layer
			if inShield(fx, fy) {
				// Outer ring vs inner shield
				edgeDist := s * 0.025
				isEdge := !inShield(fx-edgeDist, fy) || !inShield(fx+edgeDist, fy) ||
					!inShield(fx, fy-edgeDist) || !inShield(fx, fy+edgeDist)

				// Cyan-to-Indigo gradient: #22d3ee to #6d7cff
				tShield := (fy - (cy - s*0.27)) / (s * 0.54)
				if tShield < 0 {
					tShield = 0
				}
				if tShield > 1 {
					tShield = 1
				}

				if isEdge {
					// Glowing cyan border (#22d3ee)
					img.SetRGBA(x, y, color.RGBA{34, 211, 238, 255})
				} else {
					// Gradient fill: 109,124,255 -> 34,211,238
					rVal := uint8(109*(1-tShield) + 34*tShield)
					gVal := uint8(124*(1-tShield) + 211*tShield)
					bVal := uint8(255*(1-tShield) + 238*tShield)
					img.SetRGBA(x, y, color.RGBA{rVal, gVal, bVal, 255})
				}
			} else {
				img.SetRGBA(x, y, color.RGBA{bgR, bgG, bgB, 255})
			}
		}
	}

	// White checkmark inside the shield
	checkScale := s / 32.0
	drawCheck := func(x0, y0, x1, y1 float64) {
		steps := int(math.Hypot(x1-x0, y1-y0)*2 + 10)
		thickness := s * 0.022
		for i := 0; i <= steps; i++ {
			t := float64(i) / float64(steps)
			px := x0 + (x1-x0)*t
			py := y0 + (y1-y0)*t
			for dy := -int(thickness); dy <= int(thickness); dy++ {
				for dx := -int(thickness); dx <= int(thickness); dx++ {
					if float64(dx*dx+dy*dy) <= thickness*thickness {
						ix := int(px) + dx
						iy := int(py) + dy
						if ix >= 0 && ix < size && iy >= 0 && iy < size {
							img.SetRGBA(ix, iy, color.RGBA{255, 255, 255, 255})
						}
					}
				}
			}
		}
	}

	drawCheck(cx-s*0.14, cy+s*0.01, cx-s*0.03, cy+s*0.12)
	drawCheck(cx-s*0.03, cy+s*0.12, cx+s*0.15, cy-s*0.10)
	_ = checkScale

	return img
}

func main() {
	target := "build/AppIcon.icns"
	if len(os.Args) > 1 {
		target = os.Args[1]
	}
	outDir := filepath.Join(filepath.Dir(target), "icon.iconset")
	_ = os.RemoveAll(outDir)
	_ = os.MkdirAll(outDir, 0o755)

	sizes := []struct {
		name string
		px   int
	}{
		{"icon_16x16.png", 16},
		{"icon_16x16@2x.png", 32},
		{"icon_32x32.png", 32},
		{"icon_32x32@2x.png", 64},
		{"icon_128x128.png", 128},
		{"icon_128x128@2x.png", 256},
		{"icon_256x256.png", 256},
		{"icon_256x256@2x.png", 512},
		{"icon_512x512.png", 512},
		{"icon_512x512@2x.png", 1024},
	}

	for _, sz := range sizes {
		img := renderIcon(sz.px)
		f, err := os.Create(filepath.Join(outDir, sz.name))
		if err != nil {
			fmt.Printf("Error creating %s: %v\n", sz.name, err)
			continue
		}
		_ = png.Encode(f, img)
		f.Close()
	}

	_ = os.MkdirAll(filepath.Dir(target), 0o755)
	cmd := exec.Command("iconutil", "-c", "icns", outDir, "-o", target)
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Printf("iconutil failed: %v (%s)\n", err, string(out))
		os.Exit(1)
	}
	_ = os.RemoveAll(outDir)

	fmt.Printf("Generated %s successfully!\n", target)
}
