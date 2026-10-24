package main

import (
	"embed"
	"io/fs"
	"log"

	"poseavpn/pkg/guiapp"
)

//go:embed all:frontend
var frontend embed.FS

func main() {
	sub, err := fs.Sub(frontend, "frontend")
	if err != nil {
		log.Fatal(err)
	}
	if err := guiapp.Run(guiapp.RunOptions{
		Mode:      guiapp.ModeSingle,
		Title:     "PoseaVPN",
		UniqueID:  "poseavpn-single-7c1f",
		Width:     500,
		Height:    780,
		MinWidth:  440,
		MinHeight: 640,
		Frontend:  sub,
	}); err != nil {
		log.Fatal(err)
	}
}
