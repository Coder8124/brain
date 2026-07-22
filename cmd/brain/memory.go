package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/pragun/brain/internal/memory"
	"github.com/pragun/brain/internal/router"
)

// memoryCmd inspects and edits the assistant's persistent memory — the facts it
// has learned about the user across sessions.
func memoryCmd(args []string) error {
	ix, err := openEvents()
	if err != nil {
		return err
	}
	defer ix.Close()
	if err := memory.Init(ix.DB); err != nil {
		return err
	}

	if len(args) == 0 {
		mems, err := memory.All(ix.DB)
		if err != nil {
			return err
		}
		if len(mems) == 0 {
			fmt.Println("no memories yet — the assistant learns as you talk to it.")
			return nil
		}
		fmt.Printf("%d memories\n\n", len(mems))
		for _, m := range mems {
			used := ""
			if m.Uses > 0 {
				used = fmt.Sprintf(" · recalled %d×", m.Uses)
			}
			fmt.Printf("  [%d] (%-10s %.2f) %s%s\n", m.ID, m.Kind, m.Salience, m.Text, used)
		}
		return nil
	}

	switch args[0] {
	case "add":
		text := strings.TrimSpace(strings.Join(args[1:], " "))
		if text == "" {
			return fmt.Errorf("usage: brain memory add <fact>")
		}
		rt, err := openRouter()
		if err != nil {
			return err
		}
		embed, _ := rt.Model(router.T0)
		_, err = memory.Store(ix.DB, rt.Local(), embed, &memory.Memory{
			Text: text, Kind: memory.Fact, Salience: 0.7, Source: "manual", Created: time.Now().Unix(),
		})
		if err != nil {
			return err
		}
		fmt.Println("remembered.")
	case "consolidate":
		rt, err := openRouter()
		if err != nil {
			return err
		}
		d, _ := memory.Decay(ix.DB, time.Now().Unix())
		m, s, err := memory.Consolidate(ix.DB, rt)
		if err != nil {
			return err
		}
		fmt.Printf("decayed %d · merged %d duplicates · superseded %d outdated\n", d, m, s)
		return nil
	case "forget":
		id := parseID(args)
		if id == 0 {
			return fmt.Errorf("usage: brain memory forget <id>")
		}
		if err := memory.Forget(ix.DB, id); err != nil {
			return err
		}
		fmt.Println("forgotten.")
	default:
		return fmt.Errorf("usage: brain memory [add <fact> | forget <id>]")
	}
	return nil
}
