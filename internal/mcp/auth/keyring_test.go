package auth

import "testing"

type memoryStore struct {
	m map[string]string
}

func (s *memoryStore) Save(serverID, tokenJSON string) error {
	if s.m == nil {
		s.m = map[string]string{}
	}
	s.m[serverID] = tokenJSON
	return nil
}
func (s *memoryStore) Load(serverID string) (string, error) { return s.m[serverID], nil }
func (s *memoryStore) Delete(serverID string) error {
	delete(s.m, serverID)
	return nil
}

func TestMemoryStoreRoundTrip(t *testing.T) {
	s := &memoryStore{}
	if err := s.Save("srv", `{"access_token":"x"}`); err != nil {
		t.Fatal(err)
	}
	got, err := s.Load("srv")
	if err != nil {
		t.Fatal(err)
	}
	if got == "" {
		t.Fatal("expected value")
	}
}
