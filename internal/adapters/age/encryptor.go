package ageadapter

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"filippo.io/age"
)

type Encryptor struct {
	recipient age.Recipient
	identity  age.Identity
}

func NewEncryptor(passphraseFile string) (*Encryptor, error) {
	raw, err := os.ReadFile(passphraseFile)
	if err != nil {
		return nil, fmt.Errorf("reading age passphrase file %s: %w", passphraseFile, err)
	}

	passphrase := strings.TrimSpace(string(raw))
	if passphrase == "" {
		return nil, fmt.Errorf("age passphrase file %s is empty", passphraseFile)
	}

	recipient, err := age.NewScryptRecipient(passphrase)
	if err != nil {
		return nil, fmt.Errorf("creating scrypt recipient: %w", err)
	}
	identity, err := age.NewScryptIdentity(passphrase)
	if err != nil {
		return nil, fmt.Errorf("creating scrypt identity: %w", err)
	}

	return &Encryptor{
		recipient: recipient,
		identity:  identity,
	}, nil
}

func (e *Encryptor) Encrypt(ctx context.Context, plaintext []byte) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var encrypted bytes.Buffer
	writer, err := age.Encrypt(&encrypted, e.recipient)
	if err != nil {
		return nil, fmt.Errorf("creating age encrypt writer: %w", err)
	}

	if _, err := writer.Write(plaintext); err != nil {
		return nil, fmt.Errorf("writing plaintext to age stream: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("closing age stream: %w", err)
	}

	return encrypted.Bytes(), nil
}

func (e *Encryptor) Decrypt(ctx context.Context, encrypted []byte) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	reader, err := age.Decrypt(bytes.NewReader(encrypted), e.identity)
	if err != nil {
		return nil, fmt.Errorf("opening age stream: %w", err)
	}
	// Read through EOF to authenticate the entire file, including the final chunk.
	plaintext, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("reading age stream: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return plaintext, nil
}
