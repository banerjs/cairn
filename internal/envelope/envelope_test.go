package envelope

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"filippo.io/age"
)

func pqRecipient(t *testing.T) ([]age.Recipient, []age.Identity) {
	t.Helper()
	id, err := age.GenerateHybridIdentity()
	if err != nil {
		t.Fatal(err)
	}
	rec, err := age.ParseHybridRecipient(id.Recipient().String())
	if err != nil {
		t.Fatal(err)
	}
	ids, err := age.ParseIdentities(strings.NewReader(id.String() + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	return []age.Recipient{rec}, ids
}

func TestEncryptDecrypt_RoundTrip(t *testing.T) {
	rec, ids := pqRecipient(t)
	plain := []byte("hello envelope")
	cipher, err := Encrypt(plain, rec)
	if err != nil {
		t.Fatal(err)
	}
	out, err := Decrypt(cipher, ids)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != string(plain) {
		t.Fatalf("got %q", out)
	}
}

func TestEncryptWithLevel(t *testing.T) {
	rec, ids := pqRecipient(t)
	cipher, err := EncryptWithLevel([]byte("x"), rec, 1)
	if err != nil {
		t.Fatal(err)
	}
	out, err := Decrypt(cipher, ids)
	if err != nil || string(out) != "x" {
		t.Fatalf("decrypt: %v %q", err, out)
	}
}

func TestEncrypt_Errors(t *testing.T) {
	if _, err := Encrypt([]byte("x"), nil); err == nil {
		t.Fatal("expected no recipients")
	}
	if _, err := EncryptWithLevel([]byte("x"), nil, 3); err == nil {
		t.Fatal("expected no recipients")
	}
}

func TestEncryptReader_StreamRoundTrip(t *testing.T) {
	rec, ids := pqRecipient(t)
	r := strings.NewReader(strings.Repeat("a", 5000))
	pr, err := EncryptReader(r, rec, 5)
	if err != nil {
		t.Fatal(err)
	}
	cipher, err := io.ReadAll(pr)
	if err != nil {
		t.Fatal(err)
	}
	out, err := Decrypt(cipher, ids)
	if err != nil || len(out) != 5000 {
		t.Fatalf("got len %d err %v", len(out), err)
	}
}

func TestEncryptReader_NoRecipients(t *testing.T) {
	if _, err := EncryptReader(strings.NewReader("x"), nil, 3); err == nil {
		t.Fatal("expected error")
	}
}

func TestEncryptReader_DefaultLevel(t *testing.T) {
	rec, ids := pqRecipient(t)
	pr, err := EncryptReader(strings.NewReader("hi"), rec, 0)
	if err != nil {
		t.Fatal(err)
	}
	cipher, err := io.ReadAll(pr)
	if err != nil {
		t.Fatal(err)
	}
	out, err := Decrypt(cipher, ids)
	if string(out) != "hi" {
		t.Fatalf("got %q", out)
	}
}

func TestDecrypt_Errors(t *testing.T) {
	if _, err := Decrypt([]byte("garbage"), nil); err == nil {
		t.Fatal("expected error")
	}
	_, ids := pqRecipient(t)
	if _, err := Decrypt([]byte("not-age"), ids); err == nil {
		t.Fatal("expected decrypt error")
	}
}

func TestDecryptReader(t *testing.T) {
	rec, ids := pqRecipient(t)
	cipher, err := Encrypt([]byte("decrypt reader"), rec)
	if err != nil {
		t.Fatal(err)
	}
	rc, err := DecryptReader(bytes.NewReader(cipher), ids)
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	b, err := io.ReadAll(rc)
	if err != nil || string(b) != "decrypt reader" {
		t.Fatalf("got %q err %v", b, err)
	}
}

func TestDecryptReader_NoIdentities(t *testing.T) {
	if _, err := DecryptReader(bytes.NewReader([]byte("x")), nil); err == nil {
		t.Fatal("expected error")
	}
}
