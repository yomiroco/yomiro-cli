package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: gen-platform-cmds <openapi.json> <output-dir>")
		os.Exit(2)
	}
	groups, err := Walk(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "walk:", err)
		os.Exit(1)
	}
	for tag, ops := range groups {
		if err := EmitGroup(tag, ops, os.Args[2]); err != nil {
			fmt.Fprintln(os.Stderr, "emit", tag, ":", err)
			os.Exit(1)
		}
		fmt.Printf("emitted %s (%d operations)\n", tag, len(ops))
	}
}
