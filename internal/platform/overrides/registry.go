// Package overrides holds hand-written cobra subcommands that take precedence
// over the OpenAPI-generated ones. Use init() per file to register.
package overrides

import (
	"sort"
	"strings"
	"sync"

	"github.com/spf13/cobra"
)

var (
	mu    sync.Mutex
	store = map[string]*cobra.Command{} // key: "group/cmd"
)

// Register adds an override under "group/<cmd.Name()>". Last writer wins.
func Register(group string, cmd *cobra.Command) {
	mu.Lock()
	defer mu.Unlock()
	store[group+"/"+cmd.Name()] = cmd
}

// Get returns the override for (group, name) if any.
func Get(group, name string) (*cobra.Command, bool) {
	mu.Lock()
	defer mu.Unlock()
	c, ok := store[group+"/"+name]
	return c, ok
}

// AllInGroup returns all overrides for a group, sorted by command name for determinism.
func AllInGroup(group string) []*cobra.Command {
	mu.Lock()
	defer mu.Unlock()
	var out []*cobra.Command
	prefix := group + "/"
	for k, v := range store {
		if strings.HasPrefix(k, prefix) {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out
}
