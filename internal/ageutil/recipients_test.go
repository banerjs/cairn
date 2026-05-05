package ageutil

import "testing"

func TestParsePQRecipients_RejectsSSH(t *testing.T) {
	_, err := ParsePQRecipients([]string{"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIBADSSH"})
	if err == nil {
		t.Fatal("expected error for ssh-ed25519 recipient")
	}
}
