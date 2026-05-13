package main

import (
	"fmt"
	"os"
	"path/filepath"
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
	// The client.gen.go lives one dir up from the platform-cmds output dir
	// (.../platform/generated and .../platform/client/client.gen.go).
	clientFile := filepath.Join(filepath.Dir(os.Args[2]), "client", "client.gen.go")
	methods, err := LoadClientMethods(clientFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "load client methods:", err)
		os.Exit(1)
	}
	for tag, ops := range groups {
		if err := EmitGroup(tag, ops, methods, os.Args[2]); err != nil {
			fmt.Fprintln(os.Stderr, "emit", tag, ":", err)
			os.Exit(1)
		}
		fmt.Printf("emitted %s (%d operations)\n", tag, len(ops))
	}
}
