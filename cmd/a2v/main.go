package main

import (
	"fmt"
	"os"

	"github.com/Shippuurl/a2v/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "a2v: "+err.Error())
		os.Exit(1)
	}
}
