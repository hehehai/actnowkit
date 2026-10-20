package licence

import (
	"encoding/json"
	"errors"
	"sync"

	"github.com/hehehai/actnowkit/credentials"
)

// Record is what an app keeps between launches: the token and the clock
// state of docs/licensing.md §8. It is stored whole, as JSON.
type Record struct {
	Token string `json:"token,omitempty"`
	// LastSeen is the latest time seen (Unix seconds): the corrected local
	// clock at each check, the server's time after each refresh.
	LastSeen int64 `json:"lastSeen,omitempty"`
	// Offset is serverTime − local time at the last refresh, in seconds
	// (0 when within 10 minutes).
	Offset int64 `json:"offset,omitempty"`
	// Stopped is the server's last refusal that ends the licence on this
	// Mac: "activation_removed", "code_revoked", "code_expired",
	// "whitelist_not_applicable" (with StoppedVersion: the version refused).
	Stopped        string `json:"stopped,omitempty"`
	StoppedVersion string `json:"stoppedVersion,omitempty"`
	// CheckedAt is the last refresh attempt (Unix seconds) and LastError
	// its error, if it failed (offline).
	CheckedAt int64  `json:"checkedAt,omitempty"`
	LastError string `json:"lastError,omitempty"`
}

// Store keeps the Record. Load returns ErrNoRecord when there is none.
type Store interface {
	Load() (Record, error)
	Save(Record) error
	Delete() error
}

// ErrNoRecord: nothing stored (not activated on this Mac).
var ErrNoRecord = errors.New("licence: no stored licence")

// KeychainAccount is the account of the licence item.
const KeychainAccount = "licence"

// KeychainStore keeps the record in the login Keychain as credentials does
// (/usr/bin/security), under the app's own service, "<bundle id>.licence":
// never the family's shared ActNow service. File and Run are for tests.
type KeychainStore struct {
	Keychain credentials.Keychain
}

// NewKeychainStore is the store of an app.
func NewKeychainStore(bundleID string) *KeychainStore {
	return &KeychainStore{Keychain: credentials.Keychain{Service: bundleID + ".licence"}}
}

func (s *KeychainStore) Load() (Record, error) {
	raw, err := s.Keychain.Get(KeychainAccount)
	if errors.Is(err, credentials.ErrNotFound) {
		return Record{}, ErrNoRecord
	}
	if err != nil {
		return Record{}, err
	}
	var r Record
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		return Record{}, ErrNoRecord
	}
	return r, nil
}

func (s *KeychainStore) Save(r Record) error {
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	return s.Keychain.Set(KeychainAccount, "ActNow licence", string(b))
}

func (s *KeychainStore) Delete() error { return s.Keychain.Delete(KeychainAccount) }

// MemoryStore keeps the record in memory (tests, previews).
type MemoryStore struct {
	mu  sync.Mutex
	rec *Record
}

func (m *MemoryStore) Load() (Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.rec == nil {
		return Record{}, ErrNoRecord
	}
	return *m.rec, nil
}

func (m *MemoryStore) Save(r Record) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rec = &r
	return nil
}

func (m *MemoryStore) Delete() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rec = nil
	return nil
}
