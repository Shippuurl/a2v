package codex

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Session is one row of the resume picker.
type Session struct {
	ID    string
	Title string
	Model string
	CWD   string
	Time  time.Time
}

var uuidRe = regexp.MustCompile(`([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})`)

// SessionsDir is where codex stores rollout files.
func SessionsDir() string {
	if v := os.Getenv("A2V_CODEX_SESSIONS"); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".codex", "sessions")
}

// Recent returns the most recently modified sessions, newest first.
func Recent(limit int) ([]Session, error) {
	root := SessionsDir()
	var files []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable subdir: skip
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasPrefix(d.Name(), "rollout-") && strings.HasSuffix(d.Name(), ".jsonl") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	type mf struct {
		path string
		mod  time.Time
	}
	ranked := make([]mf, 0, len(files))
	for _, f := range files {
		st, err := os.Stat(f)
		if err != nil {
			continue
		}
		ranked = append(ranked, mf{f, st.ModTime()})
	}
	sort.Slice(ranked, func(i, j int) bool { return ranked[i].mod.After(ranked[j].mod) })
	if limit > 0 && len(ranked) > limit {
		ranked = ranked[:limit]
	}
	out := make([]Session, 0, len(ranked))
	for _, f := range ranked {
		s := parseSession(f.path, f.mod)
		out = append(out, s)
	}
	return out, nil
}

type record struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	Payload   struct {
		SessionID string `json:"session_id"`
		CWD       string `json:"cwd"`
		Type      string `json:"type"`
		Role      string `json:"role"`
		Model     string `json:"model"`
		Content   []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		ThreadSettings struct {
			Model string `json:"model"`
		} `json:"thread_settings"`
	} `json:"payload"`
}

// parseSession reads only as much of the rollout as it needs.
func parseSession(path string, mod time.Time) Session {
	s := Session{ID: sessionID(path), Time: mod}
	f, err := os.Open(path)
	if err != nil {
		return s
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for line := 0; sc.Scan() && line < 400; line++ {
		b := sc.Bytes()
		if len(b) == 0 || b[0] != '{' {
			continue
		}
		var r record
		if json.Unmarshal(b, &r) != nil {
			continue
		}
		switch {
		case r.Type == "session_meta":
			if r.Payload.SessionID != "" {
				s.ID = r.Payload.SessionID
			}
			if r.Payload.CWD != "" {
				s.CWD = r.Payload.CWD
			}
			if r.Payload.Model != "" {
				s.Model = r.Payload.Model
			}
			if t, err := time.Parse(time.RFC3339, r.Timestamp); err == nil {
				s.Time = t.Local()
			}
		case r.Payload.ThreadSettings.Model != "":
			if s.Model == "" {
				s.Model = r.Payload.ThreadSettings.Model
			}
		case r.Payload.Role == "user" && s.Title == "":
			var parts []string
			for _, c := range r.Payload.Content {
				if c.Text != "" {
					parts = append(parts, c.Text)
				}
			}
			text := strings.TrimSpace(strings.Join(parts, " "))
			// codex injects environment/user-instruction blocks as the first
			// "user" message; they make terrible titles.
			if text == "" || strings.HasPrefix(text, "<environment_context") ||
				strings.HasPrefix(text, "<user_instructions") || strings.HasPrefix(text, "<permissions") {
				continue
			}
			s.Title = cleanTitle(text)
		}
		if s.Title != "" && s.Model != "" && s.CWD != "" {
			break
		}
	}
	if s.Title == "" {
		s.Title = "(无标题会话)"
	}
	return s
}

func sessionID(path string) string {
	name := strings.TrimSuffix(filepath.Base(path), ".jsonl")
	if m := uuidRe.FindString(name); m != "" {
		return m
	}
	return name
}

func cleanTitle(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	s = strings.TrimPrefix(s, "#")
	if r := []rune(s); len(r) > 64 {
		s = string(r[:61]) + "…"
	}
	if s == "" {
		return ""
	}
	return s
}
