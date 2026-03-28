// Package buildinfo holds linker-injected metadata (see Makefile LDFLAGS).
package buildinfo

var (
	Version   = "dev"
	GitCommit = ""
	BuildTime = ""
	GoVersion = ""
)
