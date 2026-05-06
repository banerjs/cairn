// Package envelope implements the cairn v1 blob format: plaintext → zstd → age.
//
// Callers supply plaintext as []byte or io.Reader; ciphertext is suitable for S3 PutObject.
package envelope

import (
	"bytes"
	"fmt"
	"io"

	"filippo.io/age"
	"github.com/klauspost/compress/zstd"
)

const defaultZstdLevel = 3

var (
	newZstdWriter      = zstd.NewWriter
	newZstdReader      = zstd.NewReader
	ageEncryptFn       = age.Encrypt
	ageDecryptFn       = age.Decrypt
	copyEnvelope       = io.Copy
	zstdWriterForBytes = func(w io.Writer, level int) (io.WriteCloser, error) {
		return newZstdWriter(w, zstd.WithEncoderLevel(zstd.EncoderLevelFromZstd(level)))
	}
	zstdWriterForStream = func(w io.Writer, level int) (io.WriteCloser, error) {
		return newZstdWriter(w, zstd.WithEncoderLevel(zstd.EncoderLevelFromZstd(level)))
	}
)

// Encrypt compresses plain with zstd then encrypts to age recipients.
func Encrypt(plain []byte, recipients []age.Recipient) ([]byte, error) {
	return EncryptWithLevel(plain, recipients, defaultZstdLevel)
}

// EncryptWithLevel sets the zstd encoder level for manifest/index-sized payloads.
func EncryptWithLevel(plain []byte, recipients []age.Recipient, level int) ([]byte, error) {
	if len(recipients) == 0 {
		return nil, fmt.Errorf("envelope: no recipients")
	}
	var zbuf bytes.Buffer
	zw, err := zstdWriterForBytes(&zbuf, level)
	if err != nil {
		return nil, fmt.Errorf("envelope: zstd writer: %w", err)
	}
	if _, err := zw.Write(plain); err != nil {
		_ = zw.Close()
		return nil, fmt.Errorf("envelope: zstd write: %w", err)
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("envelope: zstd close: %w", err)
	}

	var out bytes.Buffer
	aw, err := ageEncryptFn(&out, recipients...)
	if err != nil {
		return nil, fmt.Errorf("envelope: age encrypt: %w", err)
	}
	if _, err := aw.Write(zbuf.Bytes()); err != nil {
		_ = aw.Close()
		return nil, fmt.Errorf("envelope: age write: %w", err)
	}
	if err := aw.Close(); err != nil {
		return nil, fmt.Errorf("envelope: age close: %w", err)
	}
	return out.Bytes(), nil
}

// EncryptReader streams plaintext through zstd then age, returning a ciphertext reader.
// The goroutine completes when plain is fully consumed or an error occurs.
func EncryptReader(plain io.Reader, recipients []age.Recipient, level int) (io.Reader, error) {
	if len(recipients) == 0 {
		return nil, fmt.Errorf("envelope: no recipients")
	}
	if level <= 0 {
		level = defaultZstdLevel
	}
	pr, pw := io.Pipe()
	go func() {
		err := func() error {
			aw, err := ageEncryptFn(pw, recipients...)
			if err != nil {
				return err
			}
			zw, err := zstdWriterForStream(aw, level)
			if err != nil {
				_ = aw.Close()
				return err
			}
			if _, err := copyEnvelope(zw, plain); err != nil {
				_ = zw.Close()
				_ = aw.Close()
				return err
			}
			if err := zw.Close(); err != nil {
				_ = aw.Close()
				return err
			}
			return aw.Close()
		}()
		_ = pw.CloseWithError(err)
	}()
	return pr, nil
}

// Decrypt decrypts an age blob and decompresses zstd.
func Decrypt(cipher []byte, identities []age.Identity) ([]byte, error) {
	if len(identities) == 0 {
		return nil, fmt.Errorf("envelope: no identities")
	}
	ar, err := ageDecryptFn(bytes.NewReader(cipher), identities...)
	if err != nil {
		return nil, fmt.Errorf("envelope: age decrypt: %w", err)
	}
	zr, err := newZstdReader(ar)
	if err != nil {
		return nil, fmt.Errorf("envelope: zstd reader: %w", err)
	}
	defer zr.Close()
	var out bytes.Buffer
	if _, err := copyEnvelope(&out, zr); err != nil {
		return nil, fmt.Errorf("envelope: zstd read: %w", err)
	}
	return out.Bytes(), nil
}

// DecryptReader returns a plaintext reader for streaming ciphertext.
func DecryptReader(cipher io.Reader, identities []age.Identity) (io.ReadCloser, error) {
	if len(identities) == 0 {
		return nil, fmt.Errorf("envelope: no identities")
	}
	ar, err := ageDecryptFn(cipher, identities...)
	if err != nil {
		return nil, fmt.Errorf("envelope: age decrypt: %w", err)
	}
	zr, err := newZstdReader(ar)
	if err != nil {
		return nil, fmt.Errorf("envelope: zstd reader: %w", err)
	}
	return zr.IOReadCloser(), nil
}
