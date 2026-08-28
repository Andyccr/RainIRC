// Package version identifies this p2pirc build.
// Version and Commit may be overwritten at link time:
//
//	-X github.com/Andyccr/RainIRC/internal/version.Version=0.5.1
//	-X github.com/Andyccr/RainIRC/internal/version.Commit=<git sha>
package version

const Name = "p2pirc"

// Version is the semver string (major.minor.patch).
var Version = "0.5.1"

// Commit is a short git SHA when built with make / the release workflow.
var Commit = "dev"

func String() string {
	s := Name + " " + Version
	if Commit != "" && Commit != "dev" {
		s += " " + Commit
	}
	return s
}
