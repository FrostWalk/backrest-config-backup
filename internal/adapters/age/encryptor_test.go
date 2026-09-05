package ageadapter

import (
	"bytes"
	"context"
	"errors"
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

func TestEncryptorDecrypt(t *testing.T) {
	t.Parallel()

	encryptor, err := NewEncryptor(writeTempSecretFile(t, "test passphrase\n"))
	if err != nil {
		t.Fatalf("new encryptor: %v", err)
	}
	// Keep failure-case tests inexpensive; production uses age's default work factor.
	encryptor.recipient.(*age.ScryptRecipient).SetWorkFactor(10)
	plaintext := bytes.Repeat([]byte("config data"), 10_000)
	ciphertext, err := encryptor.Encrypt(t.Context(), plaintext)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	wrongPassphrase, err := NewEncryptor(writeTempSecretFile(t, "wrong passphrase"))
	if err != nil {
		t.Fatalf("new encryptor with wrong passphrase: %v", err)
	}
	corrupted := bytes.Clone(ciphertext)
	corrupted[len(corrupted)-1] ^= 1

	for _, tt := range []struct {
		name       string
		decryptor  *Encryptor
		ciphertext []byte
		wantErr    bool
	}{
		{name: "round trip across chunks", decryptor: encryptor, ciphertext: ciphertext},
		{name: "wrong passphrase", decryptor: wrongPassphrase, ciphertext: ciphertext, wantErr: true},
		{name: "corrupted final chunk", decryptor: encryptor, ciphertext: corrupted, wantErr: true},
		{name: "truncated final chunk", decryptor: encryptor, ciphertext: ciphertext[:len(ciphertext)-16], wantErr: true},
		{name: "invalid age header", decryptor: encryptor, ciphertext: []byte("invalid"), wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			restored, err := tt.decryptor.Decrypt(t.Context(), tt.ciphertext)
			if tt.wantErr {
				if err == nil || restored != nil {
					t.Fatalf("expected decryption error without partial plaintext, got %v", err)
				}
				return
			}
			if err != nil || !bytes.Equal(restored, plaintext) {
				t.Fatalf("restore mismatch: %v", err)
			}
		})
	}
}

func TestEncryptorDecryptCanceledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	decryptor := &Encryptor{}
	plaintext, err := decryptor.Decrypt(ctx, []byte("invalid ciphertext"))
	if !errors.Is(err, context.Canceled) || plaintext != nil {
		t.Fatalf("expected canceled decryption without plaintext, got %v", err)
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
