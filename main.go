package main

import (
	"fastest-dot-com/internal/ui"
	"fmt"
	"os"
)

func main() {
	if err := ui.Start(); err != nil {
		fmt.Printf("Critical Error: %v\n", err)
		os.Exit(1)
	}
}
