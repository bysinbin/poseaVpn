//go:build ignore

package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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

			// Dark gradient background (PoseaVPN dark theme: #0a0e1a -> #16203c)
			tGrad := (fx + fy) / (s * 2)
			bgR := uint8(10 + tGrad*12)
			bgG := uint8(14 + tGrad*18)
			bgB := uint8(26 + tGrad*34)

			// Shield layer
			if inShield(fx, fy) {
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
	drawCheck := func(x0, y0, x1, y1 float64) {
		steps := int(math.Hypot(x1-x0, y1-y0)*2 + 10)
		thickness := s * 0.024
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

	return img
}

func buildIco(images []*image.RGBA) ([]byte, error) {
	var buf bytes.Buffer
	// ICO header
	_ = binary.Write(&buf, binary.LittleEndian, uint16(0)) // Reserved
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1)) // Type: 1 = ICO
	_ = binary.Write(&buf, binary.LittleEndian, uint16(len(images)))

	var pngData [][]byte
	for _, img := range images {
		var p bytes.Buffer
		if err := png.Encode(&p, img); err != nil {
			return nil, err
		}
		pngData = append(pngData, p.Bytes())
	}

	offset := uint32(6 + 16*len(images))
	for i, img := range images {
		b := img.Bounds()
		w := uint8(b.Dx())
		if b.Dx() >= 256 {
			w = 0
		}
		h := uint8(b.Dy())
		if b.Dy() >= 256 {
			h = 0
		}
		size := uint32(len(pngData[i]))

		buf.WriteByte(w)
		buf.WriteByte(h)
		buf.WriteByte(0) // Colors
		buf.WriteByte(0) // Reserved
		_ = binary.Write(&buf, binary.LittleEndian, uint16(1))  // Planes
		_ = binary.Write(&buf, binary.LittleEndian, uint16(32)) // BitCount
		_ = binary.Write(&buf, binary.LittleEndian, size)
		_ = binary.Write(&buf, binary.LittleEndian, offset)
		offset += size
	}

	for _, p := range pngData {
		buf.Write(p)
	}

	return buf.Bytes(), nil
}

func main() {
	fmt.Println("[*] Generating PoseaVPN icons (PNG & ICO)...")

	// Render high-res 512x512 master PNG
	icon512 := renderIcon(512)

	// Render multi-res for Windows ICO
	icoSizes := []int{256, 128, 64, 48, 32, 24, 16}
	var icoImages []*image.RGBA
	for _, sz := range icoSizes {
		icoImages = append(icoImages, renderIcon(sz))
	}
	icoBytes, err := buildIco(icoImages)
	if err != nil {
		fmt.Printf("Error creating ICO: %v\n", err)
		os.Exit(1)
	}

	destinations := []string{
		"build",
		"cmd/poseavpn-multi/build",
		"cmd/poseavpn-single/build",
	}

	for _, dest := range destinations {
		_ = os.MkdirAll(dest, 0o755)
		_ = os.MkdirAll(filepath.Join(dest, "windows"), 0o755)

		// 1. Save appicon.png (512x512)
		pngPath := filepath.Join(dest, "appicon.png")
		f, err := os.Create(pngPath)
		if err == nil {
			_ = png.Encode(f, icon512)
			f.Close()
			fmt.Printf("  -> Created: %s\n", pngPath)
		}

		// 2. Save windows/icon.ico
		icoPath := filepath.Join(dest, "windows", "icon.ico")
		if err := os.WriteFile(icoPath, icoBytes, 0o644); err == nil {
			fmt.Printf("  -> Created: %s\n", icoPath)
		}
	}

	// If macOS iconutil is available, also generate AppIcon.icns
	if runtime.GOOS == "darwin" || hasCommand("iconutil") {
		generateMacIcns()
	}

	fmt.Println("[OK] All icons generated successfully!")
}

func hasCommand(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func generateMacIcns() {
	target := "build/AppIcon.icns"
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
			continue
		}
		_ = png.Encode(f, img)
		f.Close()
	}

	_ = os.MkdirAll(filepath.Dir(target), 0o755)
	cmd := exec.Command("iconutil", "-c", "icns", outDir, "-o", target)
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Printf("iconutil failed: %v (%s)\n", err, string(out))
		return
	}
	_ = os.RemoveAll(outDir)
	fmt.Printf("  -> Created: %s\n", target)
}
