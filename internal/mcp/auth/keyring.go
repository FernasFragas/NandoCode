package auth

import (
	"fmt"

	"github.com/zalando/go-keyring"
)

type TokenStore interface {
	Save(serverID, tokenJSON string) error
	Load(serverID string) (string, error)
	Delete(serverID string) error
}

type KeyringStore struct {
	Service string
}

func NewKeyringStore() *KeyringStore {
	return &KeyringStore{Service: "nandocodego-mcp"}
}

func (s *KeyringStore) key(serverID string) string {
	return "token:" + serverID
}

func (s *KeyringStore) Save(serverID, tokenJSON string) error {
	if s == nil {
		return fmt.Errorf("nil keyring store")
	}
	return keyring.Set(s.Service, s.key(serverID), tokenJSON)
}

func (s *KeyringStore) Load(serverID string) (string, error) {
	if s == nil {
		return "", fmt.Errorf("nil keyring store")
	}
	return keyring.Get(s.Service, s.key(serverID))
}

func (s *KeyringStore) Delete(serverID string) error {
	if s == nil {
		return fmt.Errorf("nil keyring store")
	}
	return keyring.Delete(s.Service, s.key(serverID))
}
