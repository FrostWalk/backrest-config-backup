package ageadapter

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"filippo.io/age"
)

func TestEncryptorEncryptRoundTripWithScryptPassphrase(t *testing.T) {
	t.Parallel()

	const passphrase = "correct horse battery staple"
	secretPath := writeTempSecretFile(t, passphrase+"\n")

	encryptor, err := NewEncryptor(secretPath)
	if err != nil {
		t.Fatalf("new encryptor: %v", err)
	}

	plaintext := []byte(`{"setting":"value","enabled":true}`)
	ciphertext, err := encryptor.Encrypt(context.Background(), plaintext)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if len(ciphertext) == 0 {
		t.Fatalf("expected ciphertext")
	}

	identity, err := age.NewScryptIdentity(passphrase)
	if err != nil {
		t.Fatalf("new scrypt identity: %v", err)
	}

	reader, err := age.Decrypt(bytes.NewReader(ciphertext), identity)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	decrypted, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read decrypted: %v", err)
	}
	if !bytes.Equal(decrypted, plaintext) {
		t.Fatalf("roundtrip mismatch: got %q want %q", string(decrypted), string(plaintext))
	}
}

func TestNewEncryptorFailsOnEmptyPassphraseFile(t *testing.T) {
	t.Parallel()

	secretPath := writeTempSecretFile(t, "\n")

	_, err := NewEncryptor(secretPath)
	if err == nil {
		t.Fatalf("expected error for empty passphrase file")
	}
}

func writeTempSecretFile(t *testing.T, content string) string {
	t.Helper()

	secretPath := filepath.Join(t.TempDir(), "age_passphrase.txt")
	if err := os.WriteFile(secretPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write secret file: %v", err)
	}
	return secretPath
}
