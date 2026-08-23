// Package version identifies this p2pirc build.
package version

const (
	Major = 0
	Minor = 4
	Patch = 3
	Name  = "p2pirc"
)

// Version is the semver string (major.minor.patch).
const Version = "0.4.3"

func String() string {
	return Name + " " + Version
}
