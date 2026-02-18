package main

import (
	"fmt"
	"os"

	"github.com/AugustDG/hopper/internal/app"
)

func main() {
	if err := app.Run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "hopper:", err)
		os.Exit(1)
	}
}
