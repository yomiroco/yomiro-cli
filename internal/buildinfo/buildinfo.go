// Package buildinfo holds release metadata set via -ldflags at link time.
package buildinfo

var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)
