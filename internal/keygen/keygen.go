package keygen

import (
	"fmt"
	"os"

	"filippo.io/age"
)

// Swapped in tests (see keygen_test.go).
var (
	osStat                 = os.Stat
	osWriteFile            = os.WriteFile
	generateHybridIdentity = age.GenerateHybridIdentity
)

// WriteNewHybridIdentity generates a PQ hybrid identity and writes it to path with mode 0600.
//
// It refuses to overwrite an existing file. The matching recipient line is returned for config.toml.
func WriteNewHybridIdentity(path string) (recipient string, err error) {
	if _, err := osStat(path); err == nil {
		return "", fmt.Errorf("keygen: %s already exists", path)
	} else if !os.IsNotExist(err) {
		return "", err
	}
	id, err := generateHybridIdentity()
	if err != nil {
		return "", err
	}
	line := id.String() + "\n"
	if err := osWriteFile(path, []byte(line), 0o600); err != nil {
		return "", err
	}
	return id.Recipient().String(), nil
}
