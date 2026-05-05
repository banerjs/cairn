// Package format holds the canonical FORMAT.md specification text embedded in the binary.
package format

import _ "embed"

// Markdown is the full on-disk / S3 format specification (v1).
//
//go:embed FORMAT.md
var Markdown string
