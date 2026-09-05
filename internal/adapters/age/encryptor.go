package ageadapter

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"

	"filippo.io/age"
)

type Encryptor struct {
	recipient age.Recipient
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

	return &Encryptor{
		recipient: recipient,
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
