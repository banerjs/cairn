// Package version holds release metadata for the cairn CLI.
package version

// Name is the binary name shown in help output.
const Name = "cairn"

// Version is the semver release string (overridden at link time with -ldflags).
var Version = "0.1.0"

// ManifestSchemas lists manifest/index schema strings this binary understands.
var ManifestSchemas = []string{"cairn.manifest.v1", "cairn.index.v1"}
