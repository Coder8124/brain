package main

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/pragun/brain/internal/record"
)

// runRecord captures a study session and turns it into notes. It records until
// interrupted (^C) — the CLI stand-in for the app's start/stop hotkey — then
// titles, summarises, and files what it saw.
func runRecord(name string, noVideo bool) error {
	ix, err := openEvents()
	if err != nil {
		return err
	}
	defer ix.Close()
	rt, err := openRouter()
	if err != nil {
		return err
	}

	rec := record.NewRecorder(scratchDir(ix.Vault))
	withVideo := !noVideo && record.FFmpegAvailable()

	if err := rec.Start(withVideo); err != nil {
		return err
	}
	if withVideo {
		fmt.Println("● recording screen + video. ^C to stop.")
	} else {
		fmt.Println("● recording screen (notes only; install ffmpeg for video). ^C to stop.")
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	fmt.Println("\n· processing …")
	session := rec.Stop()

	res, err := record.Process(rt, ix.DB, ix.Vault, session, strings.TrimSpace(name))
	if err != nil {
		return err
	}

	fmt.Printf("\n✓ %s\n", res.Title)
	fmt.Printf("  notes  → %s\n", res.NotePath)
	if res.VideoPath != "" {
		fmt.Printf("  video  → %s\n", res.VideoPath)
	}
	if res.Cards > 0 {
		fmt.Printf("  cards  → %d added — `brain tutor review`\n", res.Cards)
	}
	fmt.Println("\nrun `brain index` to make it searchable and quizzable")
	return nil
}
