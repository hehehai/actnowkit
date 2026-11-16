package feedback

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"

	"github.com/hehehai/actnowkit/family"
)

// HistoryEntry is a feedback this Mac sent: the site's item and the token
// that lets this Mac read it (it grants nothing else).
type HistoryEntry struct {
	Item
	ViewToken string `json:"viewToken"`
	// SeenMessages: how many of the thread's messages the user has seen
	// (Unread compares).
	SeenMessages int `json:"seenMessages,omitempty"`
}

// Unread: the thread has messages the user has not opened.
func (e HistoryEntry) Unread() bool { return e.Messages > e.SeenMessages }

// Ref of the entry.
func (e HistoryEntry) Ref() Ref { return Ref{ID: e.ID, ViewToken: e.ViewToken} }

type historyFile struct {
	Schema string         `json:"schema"`
	Items  []HistoryEntry `json:"items"`
}

// MaxHistory entries are kept (the newest).
const MaxHistory = 200

func (m *Manager) historyPath() string { return filepath.Join(m.cfg.Dir, "history.json") }

func (m *Manager) loadHistory() []HistoryEntry {
	if m.hist != nil {
		return append([]HistoryEntry(nil), m.hist...)
	}
	var h historyFile
	if err := family.Load(m.historyPath(), HistoryKind, 1, &h); err != nil {
		var ce *family.CorruptError
		if errors.As(err, &ce) {
			_ = family.Quarantine(m.historyPath())
		}
		return nil
	}
	m.hist = h.Items
	return append([]HistoryEntry(nil), h.Items...)
}

func (m *Manager) saveHistory(items []HistoryEntry) error {
	sort.SliceStable(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	if len(items) > MaxHistory {
		items = items[:MaxHistory]
	}
	if err := family.WriteJSON(m.historyPath(), historyFile{Schema: family.SchemaOf(HistoryKind, 1), Items: items}); err != nil {
		m.hist = nil
		return err
	}
	m.hist = append([]HistoryEntry(nil), items...)
	return nil
}

// remember adds or replaces an entry.
func (m *Manager) remember(e HistoryEntry) error {
	m.histMu.Lock()
	defer m.histMu.Unlock()
	items := m.loadHistory()
	for i := range items {
		if items[i].ID == e.ID {
			e.SeenMessages = items[i].SeenMessages
			items[i] = e
			return m.saveHistory(items)
		}
	}
	return m.saveHistory(append(items, e))
}

// History is this Mac's feedback, newest first, as last refreshed (from
// memory after the first read: cheap enough for the UI thread).
func (m *Manager) History() []HistoryEntry {
	m.histMu.Lock()
	defer m.histMu.Unlock()
	items := m.loadHistory()
	sort.SliceStable(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	return items
}

// Unread counts the entries with unread messages (a badge on Feedback).
func (m *Manager) Unread() int {
	n := 0
	for _, e := range m.History() {
		if e.Unread() {
			n++
		}
	}
	return n
}

// Refresh asks the site for the statuses and messages of the history.
func (m *Manager) Refresh(ctx context.Context) ([]HistoryEntry, error) {
	items := m.History()
	if len(items) == 0 {
		return nil, nil
	}
	refs := make([]Ref, len(items))
	for i, e := range items {
		refs[i] = e.Ref()
	}
	got, err := m.client.List(ctx, refs)
	if err != nil {
		return items, err
	}
	m.histMu.Lock()
	defer m.histMu.Unlock()
	items = m.loadHistory()
	for i := range items {
		for _, it := range got {
			if it.ID == items[i].ID {
				if it.WebURL == "" {
					it.WebURL = items[i].WebURL
				}
				items[i].Item = it
			}
		}
	}
	if err := m.saveHistory(items); err != nil {
		return items, err
	}
	return items, nil
}

func (m *Manager) entry(id string) (HistoryEntry, bool) {
	for _, e := range m.History() {
		if e.ID == id {
			return e, true
		}
	}
	return HistoryEntry{}, false
}

// Thread reads one feedback of the history with its messages, and marks
// them seen.
func (m *Manager) Thread(ctx context.Context, id string) (Thread, error) {
	e, ok := m.entry(id)
	if !ok {
		return Thread{}, os.ErrNotExist
	}
	t, err := m.client.Get(ctx, e.Ref())
	if err != nil {
		return t, err
	}
	m.histMu.Lock()
	items := m.loadHistory()
	for i := range items {
		if items[i].ID == id {
			if t.Item.ID != "" {
				if t.Item.WebURL == "" {
					t.Item.WebURL = items[i].WebURL
				}
				items[i].Item = t.Item
			}
			items[i].SeenMessages = max(len(t.Messages), items[i].Messages)
		}
	}
	err = m.saveHistory(items)
	m.histMu.Unlock()
	return t, err
}

// Reply adds the user's message to a feedback of the history.
func (m *Manager) Reply(ctx context.Context, id, body string) (Message, error) {
	e, ok := m.entry(id)
	if !ok {
		return Message{}, os.ErrNotExist
	}
	msg, err := m.client.Reply(ctx, e.Ref(), body)
	if err != nil {
		return msg, err
	}
	// The user's own message is not unread.
	m.histMu.Lock()
	defer m.histMu.Unlock()
	items := m.loadHistory()
	for i := range items {
		if items[i].ID == id {
			items[i].Messages++
			items[i].SeenMessages = max(items[i].SeenMessages+1, items[i].Messages)
		}
	}
	return msg, m.saveHistory(items)
}

// Forget removes an entry from this Mac's history (the site keeps it).
func (m *Manager) Forget(id string) error {
	m.histMu.Lock()
	defer m.histMu.Unlock()
	items := m.loadHistory()
	out := items[:0]
	for _, e := range items {
		if e.ID != id {
			out = append(out, e)
		}
	}
	return m.saveHistory(out)
}
