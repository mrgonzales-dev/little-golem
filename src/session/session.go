// Package session keeps the project's one chat session on disk, in
// .little-golem/session.json under the project folder, so little-golem
// picks the conversation up where it was left.
package session

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"little-golem/src/config"
	"little-golem/src/llama"
	"little-golem/src/model"
)

const (
	dirName  = ".little-golem"
	fileName = "session.json"
)

// Session is what gets saved: the API history, the transcript and the
// model that was running.
type Session struct {
	Model     string
	TokenUsed int
	History   []llama.ChatMessage
	Entries   []model.Entry
}

func dir() string  { return filepath.Join(config.WorkDir, dirName) }
func path() string { return filepath.Join(dir(), fileName) }

// Save writes m's session atomically, creating .little-golem/ (git-ignored
// by a "*" .gitignore inside it) on first use. It does nothing without a
// project folder, or for an empty session.
func Save(m *model.App) error {
	if config.WorkDir == "" || (len(m.History) == 0 && len(m.Entries) == 0) {
		return nil
	}
	raw, err := json.Marshal(Session{m.ModelName(), m.TokenUsed, m.History, m.Entries})
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir(), 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(dir(), ".gitignore")); err != nil {
		os.WriteFile(filepath.Join(dir(), ".gitignore"), []byte("*\n"), 0o644)
	}
	tmp := path() + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path())
}

// Clear deletes the saved session.
func Clear() error {
	if config.WorkDir == "" {
		return nil
	}
	if err := os.Remove(path()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// Load reads the saved session, or returns nil when there is none. A file
// that cannot be parsed is moved to session.json.bad and reported.
func Load() (*Session, error) {
	raw, err := os.ReadFile(path())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var s Session
	if err := json.Unmarshal(raw, &s); err != nil {
		os.Rename(path(), path()+".bad")
		return nil, errors.New("corrupt session moved to " + path() + ".bad: " + err.Error())
	}
	s.clean()
	return &s, nil
}

// ModelIdx is the index in config.Models of the saved model, 0 if unknown.
func (s *Session) ModelIdx() int {
	for i, md := range config.Models {
		if md.Name == s.Model {
			return i
		}
	}
	return 0
}

// Apply installs the session into m.
func (s *Session) Apply(m *model.App) {
	m.ModelIdx, m.TokenUsed = s.ModelIdx(), s.TokenUsed
	m.History, m.Entries = s.History, s.Entries
}

// clean drops what a quit mid-turn left half done: a tool-call message
// without all its results (the chat template rejects it, and everything
// after it) and transcript entries that were still streaming.
func (s *Session) clean() {
	h := s.History
	for i := 0; i < len(h); i++ {
		if n := len(h[i].ToolCalls); h[i].Role == "assistant" && n > 0 {
			j := i + 1
			for j < len(h) && h[j].Role == "tool" {
				j++
			}
			if j-i-1 < n {
				h = h[:i]
				break
			}
			i = j - 1
		}
	}
	s.History = h
	kept := s.Entries[:0]
	for _, e := range s.Entries {
		if !e.Streaming {
			kept = append(kept, e)
		}
	}
	s.Entries = kept
}
