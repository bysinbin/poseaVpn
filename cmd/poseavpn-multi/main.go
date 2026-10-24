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
		Mode:      guiapp.ModeMulti,
		Title:     "PoseaVPN — Çoklu",
		UniqueID:  "poseavpn-multi-7c1f",
		Width:     1040,
		Height:    700,
		MinWidth:  820,
		MinHeight: 560,
		Frontend:  sub,
	}); err != nil {
		log.Fatal(err)
	}
}
