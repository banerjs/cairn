package main

import (
	"strings"
)

// reorderFlagsBeforePositionals reshuffles args so the standard flag package parses
// flags that appear after positional arguments (GNU-style interspersed usage).
//
// Tokens after "--" are all treated as positional. Otherwise, any "-..." token begins
// a flag; valued flags consume the following token when no "=" is present in the flag token.
func reorderFlagsBeforePositionals(args []string, valuedNames map[string]struct{}) []string {
	i := 0
	var flags []string
	var positionals []string
	for i < len(args) {
		a := args[i]
		if a == "--" {
			for j := i + 1; j < len(args); j++ {
				positionals = append(positionals, args[j])
			}
			break
		}
		if a == "-" || !strings.HasPrefix(a, "-") {
			positionals = append(positionals, a)
			i++
			continue
		}
		name, _, eq := splitFlagToken(a)
		if eq {
			flags = append(flags, a)
			i++
			continue
		}
		if _, need := valuedNames[name]; need && i+1 < len(args) {
			flags = append(flags, a, args[i+1])
			i += 2
			continue
		}
		flags = append(flags, a)
		i++
	}
	return append(flags, positionals...)
}

func splitFlagToken(a string) (name string, value string, equals bool) {
	if len(a) < 2 || a[0] != '-' {
		return "", "", false
	}
	body := strings.TrimPrefix(a[1:], "-")
	idx := strings.IndexByte(body, '=')
	if idx >= 0 {
		return body[:idx], body[idx+1:], true
	}
	return body, "", false
}

// Per-command flag names that consume a separate argv token (not bool; not -name=value only).
var (
	commandValuedRestoreFlags = map[string]struct{}{
		"config":      {},
		"target":      {},
		"parallelism": {},
	}
	commandValuedSnapshotsFlags = map[string]struct{}{
		"config": {},
		"host":   {},
	}
	commandValuedVerifyFlags = map[string]struct{}{
		"config": {},
		"sample": {},
	}
	commandValuedPruneFlags = map[string]struct{}{
		"config":       {},
		"keep-last":    {},
		"keep-monthly": {},
	}
	commandValuedStatusFlags = map[string]struct{}{
		"config": {},
		"host":   {},
	}
	commandValuedExportRecoveryKitFlags = map[string]struct{}{
		"output": {},
		"config": {},
	}
)
