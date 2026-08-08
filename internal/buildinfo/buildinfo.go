// Package buildinfo holds version metadata injected via -ldflags at build time.
package buildinfo

var (
	Version   = "dev"
	Commit    = "unknown"
	BuildTime = "unknown"
)
