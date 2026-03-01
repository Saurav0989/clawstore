package main

import (
	"os"

	"github.com/saurav0989/clawstore/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
