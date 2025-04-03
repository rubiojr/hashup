package crypto

import (
	"bytes"
	"fmt"
	"io"

	"filippo.io/age"
	"filippo.io/age/armor"
)

type ageX25519Machine struct {
	recipient age.Recipient
	identity  age.Identity
}

func NewAge(privateKey string) (Machine, error) {
	identity, err := age.ParseX25519Identity(privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	return &ageX25519Machine{
		recipient: identity.Recipient(),
		identity:  identity,
	}, nil
}

func GenerateAgeKeyPair() (publicKey string, privateKey string, err error) {
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		return "", "", err
	}

	return identity.Recipient().String(), identity.String(), nil
}

func DerivePublicKey(privateKey string) (string, error) {
	identity, err := age.ParseX25519Identity(privateKey)
	if err != nil {
		return "", fmt.Errorf("failed to parse private key: %w", err)
	}

	return identity.Recipient().String(), nil
}

func (a *ageX25519Machine) Encrypt(data []byte) ([]byte, error) {
	var buf bytes.Buffer

	armorWriter := armor.NewWriter(&buf)
	ageWriter, err := age.Encrypt(armorWriter, a.recipient)
	if err != nil {
		return nil, err
	}

	if _, err := ageWriter.Write(data); err != nil {
		return nil, err
	}

	if err := ageWriter.Close(); err != nil {
		return nil, err
	}

	if err := armorWriter.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func (a *ageX25519Machine) Decrypt(data []byte) ([]byte, error) {
	reader := bytes.NewReader(data)

	armorReader := armor.NewReader(reader)
	ageReader, err := age.Decrypt(armorReader, a.identity)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, ageReader); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
