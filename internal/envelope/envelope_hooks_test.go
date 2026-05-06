package envelope

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"filippo.io/age"
	"github.com/klauspost/compress/zstd"
)

type errWriteCloser struct {
	writeErr error
	closeErr error
}

func (e errWriteCloser) Write([]byte) (int, error) { return 0, e.writeErr }
func (e errWriteCloser) Close() error              { return e.closeErr }

type closeErrWriter struct{}

func (closeErrWriter) Write(p []byte) (int, error) { return len(p), nil }
func (closeErrWriter) Close() error                { return errors.New("zw-close") }

func TestEncryptWithLevel_AgeEncryptError(t *testing.T) {
	prev := ageEncryptFn
	prevZW := zstdWriterForBytes
	defer func() { zstdWriterForBytes = prevZW }()
	ageEncryptFn = func(io.Writer, ...age.Recipient) (io.WriteCloser, error) {
		return nil, errors.New("age encrypt")
	}
	defer func() { ageEncryptFn = prev }()
	rec, _ := pqRecipient(t)
	if _, err := EncryptWithLevel([]byte("x"), rec, 3); err == nil {
		t.Fatal("expected error")
	}
}

func TestEncryptWithLevel_AgeWriteAndCloseErrors(t *testing.T) {
	rec, _ := pqRecipient(t)

	prev := ageEncryptFn
	prevZW := zstdWriterForBytes
	defer func() { ageEncryptFn = prev }()
	defer func() { zstdWriterForBytes = prevZW }()
	zstdWriterForBytes = func(io.Writer, int) (io.WriteCloser, error) {
		return errWriteCloser{}, nil
	}
	ageEncryptFn = func(io.Writer, ...age.Recipient) (io.WriteCloser, error) {
		return errWriteCloser{writeErr: errors.New("age write")}, nil
	}
	if _, err := EncryptWithLevel([]byte("x"), rec, 3); err == nil || !strings.Contains(err.Error(), "age write") {
		t.Fatalf("err=%v", err)
	}

	ageEncryptFn = func(io.Writer, ...age.Recipient) (io.WriteCloser, error) {
		return errWriteCloser{closeErr: errors.New("age close")}, nil
	}
	if _, err := EncryptWithLevel([]byte("x"), rec, 3); err == nil || !strings.Contains(err.Error(), "age close") {
		t.Fatalf("err=%v", err)
	}
}

func TestEncryptWithLevel_ZstdHookErrors(t *testing.T) {
	rec, _ := pqRecipient(t)
	prev := zstdWriterForBytes
	defer func() { zstdWriterForBytes = prev }()

	zstdWriterForBytes = func(io.Writer, int) (io.WriteCloser, error) {
		return nil, errors.New("zw")
	}
	if _, err := EncryptWithLevel([]byte("x"), rec, 3); err == nil || !strings.Contains(err.Error(), "zstd writer") {
		t.Fatalf("err=%v", err)
	}

	zstdWriterForBytes = func(io.Writer, int) (io.WriteCloser, error) {
		return errWriteCloser{writeErr: errors.New("zw-write")}, nil
	}
	if _, err := EncryptWithLevel([]byte("x"), rec, 3); err == nil || !strings.Contains(err.Error(), "zstd write") {
		t.Fatalf("err=%v", err)
	}

	zstdWriterForBytes = func(io.Writer, int) (io.WriteCloser, error) {
		return errWriteCloser{closeErr: errors.New("zw-close")}, nil
	}
	if _, err := EncryptWithLevel([]byte("x"), rec, 3); err == nil || !strings.Contains(err.Error(), "zstd close") {
		t.Fatalf("err=%v", err)
	}
}

func TestEncryptReader_HookErrors(t *testing.T) {
	rec, _ := pqRecipient(t)
	prevAge := ageEncryptFn
	prevZW := zstdWriterForStream
	defer func() { ageEncryptFn = prevAge }()
	defer func() { zstdWriterForStream = prevZW }()
	ageEncryptFn = func(io.Writer, ...age.Recipient) (io.WriteCloser, error) {
		return nil, errors.New("enc")
	}
	r, err := EncryptReader(strings.NewReader("x"), rec, 3)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadAll(r); err == nil {
		t.Fatal("expected pipe error")
	}

	ageEncryptFn = prevAge
	prevCopy := copyEnvelope
	copyEnvelope = func(io.Writer, io.Reader) (int64, error) { return 0, errors.New("copy") }
	defer func() { copyEnvelope = prevCopy }()
	r, err = EncryptReader(strings.NewReader("x"), rec, 3)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadAll(r); err == nil {
		t.Fatal("expected pipe copy error")
	}

	copyEnvelope = prevCopy
	zstdWriterForStream = func(io.Writer, int) (io.WriteCloser, error) {
		return nil, errors.New("zw")
	}
	r, err = EncryptReader(strings.NewReader("x"), rec, 3)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadAll(r); err == nil {
		t.Fatal("expected stream zstd writer error")
	}

	zstdWriterForStream = func(io.Writer, int) (io.WriteCloser, error) {
		return closeErrWriter{}, nil
	}
	r, err = EncryptReader(strings.NewReader("x"), rec, 3)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadAll(r); err == nil {
		t.Fatal("expected stream zstd close error")
	}
}

func TestDecrypt_HookErrors(t *testing.T) {
	_, ids := pqRecipient(t)

	prevDec := ageDecryptFn
	defer func() { ageDecryptFn = prevDec }()
	ageDecryptFn = func(io.Reader, ...age.Identity) (io.Reader, error) {
		return nil, errors.New("dec")
	}
	if _, err := Decrypt([]byte("x"), ids); err == nil || !strings.Contains(err.Error(), "age decrypt") {
		t.Fatalf("err=%v", err)
	}

	ageDecryptFn = func(io.Reader, ...age.Identity) (io.Reader, error) {
		return bytes.NewReader([]byte("bad-zstd")), nil
	}
	if _, err := Decrypt([]byte("x"), ids); err == nil || !strings.Contains(err.Error(), "zstd") {
		t.Fatalf("err=%v", err)
	}

	prevReader := newZstdReader
	defer func() { newZstdReader = prevReader }()
	newZstdReader = func(io.Reader, ...zstd.DOption) (*zstd.Decoder, error) {
		return nil, errors.New("zstd reader")
	}
	if _, err := Decrypt([]byte("x"), ids); err == nil || !strings.Contains(err.Error(), "zstd reader") {
		t.Fatalf("err=%v", err)
	}
}

func TestDecryptReader_HookErrors(t *testing.T) {
	_, ids := pqRecipient(t)
	prevDec := ageDecryptFn
	prevReader := newZstdReader
	defer func() { ageDecryptFn = prevDec }()
	defer func() { newZstdReader = prevReader }()
	ageDecryptFn = func(io.Reader, ...age.Identity) (io.Reader, error) {
		return nil, errors.New("dec")
	}
	if _, err := DecryptReader(bytes.NewReader([]byte("x")), ids); err == nil {
		t.Fatal("expected decrypt error")
	}

	ageDecryptFn = prevDec
	newZstdReader = func(io.Reader, ...zstd.DOption) (*zstd.Decoder, error) {
		return nil, errors.New("zr")
	}
	rec, ids2 := pqRecipient(t)
	cipher, err := Encrypt([]byte("hi"), rec)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecryptReader(bytes.NewReader(cipher), ids2); err == nil || !strings.Contains(err.Error(), "zstd reader") {
		t.Fatalf("err=%v", err)
	}
}
