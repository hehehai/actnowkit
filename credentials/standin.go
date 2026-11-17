package credentials

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// store keeps generic-password items (service, account → secret) in
// memory, and in file when it has one (0600, rewritten whole).
type store struct {
	mu    sync.Mutex
	file  string
	items map[string]string // service + "\x00" + account
}

var (
	storesMu sync.Mutex
	stores   = map[string]*store{}
)

// standIn is a Keychain Run answering what credentials asks
// security (find-generic-password -g, add-generic-password -U through
// -i, delete-generic-password) from the store of file ("" memory).
func standIn(file string) func(stdin string, args ...string) ([]byte, []byte, error) {
	storesMu.Lock()
	s := stores[file]
	if s == nil {
		s = &store{file: file, items: map[string]string{}}
		s.load()
		stores[file] = s
	}
	storesMu.Unlock()
	return s.run
}

func (s *store) load() {
	if s.file == "" {
		return
	}
	if b, err := os.ReadFile(s.file); err == nil {
		json.Unmarshal(b, &s.items)
	}
}

func (s *store) save() error {
	if s.file == "" {
		return nil
	}
	b, err := json.Marshal(s.items)
	if err != nil {
		return err
	}
	os.MkdirAll(filepath.Dir(s.file), 0o700)
	return os.WriteFile(s.file, b, 0o600)
}

func (s *store) run(stdin string, args ...string) ([]byte, []byte, error) {
	if len(args) == 1 && args[0] == "-i" { // interactive: one add-generic-password line
		args = standInSplit(strings.TrimSpace(stdin))
	}
	if len(args) == 0 {
		return nil, nil, errors.New("keychain stand-in: no command")
	}
	opt := func(name string) string {
		for i := 1; i+1 < len(args); i++ {
			if args[i] == name {
				return args[i+1]
			}
		}
		return ""
	}
	key := opt("-s") + "\x00" + opt("-a")
	s.mu.Lock()
	defer s.mu.Unlock()
	switch args[0] {
	case "find-generic-password":
		v, ok := s.items[key]
		if !ok {
			return nil, []byte("security: The specified item could not be found in the keychain."), ErrNotFound
		}
		return nil, []byte("password: 0x" + strings.ToUpper(hex.EncodeToString([]byte(v))) + "\n"), nil
	case "add-generic-password":
		b, err := hex.DecodeString(opt("-X"))
		if err != nil {
			return nil, nil, fmt.Errorf("keychain stand-in: %w", err)
		}
		s.items[key] = string(b)
		return nil, nil, s.save()
	case "delete-generic-password":
		if _, ok := s.items[key]; !ok {
			return nil, nil, ErrNotFound
		}
		delete(s.items, key)
		return nil, nil, s.save()
	}
	return nil, nil, fmt.Errorf("keychain stand-in: %s is not supported", args[0])
}

// standInSplit splits a security -i line into words ("…" kept whole).
func standInSplit(line string) []string {
	var out []string
	var w strings.Builder
	in, has := false, false
	for _, r := range line {
		switch {
		case r == '"':
			in, has = !in, true
		case r == ' ' && !in:
			if has {
				out = append(out, w.String())
				w.Reset()
				has = false
			}
		default:
			w.WriteRune(r)
			has = true
		}
	}
	if has {
		out = append(out, w.String())
	}
	return out
}
