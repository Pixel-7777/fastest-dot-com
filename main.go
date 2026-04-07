package main

import (
	"fastest-dot-com/internal/ui"
	"fmt"
	"os"
	"runtime/debug"
)

func main() {
	debug.SetGCPercent(20)
	if err := ui.Start(); err != nil {
		fmt.Printf("Critical Error: %v\n", err)
		os.Exit(1)
	}
}
