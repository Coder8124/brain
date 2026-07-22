package main

import (
	"fmt"

	"github.com/pragun/brain/internal/memory"
	"github.com/pragun/brain/internal/router"
)

// runBench evaluates the memory's retrieval recall against a LongMemEval file.
func runBench(path string, n, k int) error {
	rt, err := openRouter()
	if err != nil {
		return err
	}
	embed, _ := rt.Model(router.T0)

	fmt.Printf("· LongMemEval retrieval recall@%d over %d instances of %s\n", k, n, path)
	results, err := memory.RunLongMemEval(rt.Local(), embed, path, k, n, func(done, total int) {
		fmt.Printf("\r  %d/%d …", done, total)
	})
	if err != nil {
		return err
	}
	fmt.Printf("\r%40s\r", "")

	fmt.Printf("\n%-26s %6s  %s\n", "category", "recall", "n")
	for _, r := range results {
		bar := ""
		filled := int(r.Recall() * 20)
		for i := 0; i < 20; i++ {
			if i < filled {
				bar += "█"
			} else {
				bar += "·"
			}
		}
		fmt.Printf("%-26s %5.1f%%  %-4d %s\n", r.Category, r.Recall()*100, r.N, bar)
	}
	return nil
}
