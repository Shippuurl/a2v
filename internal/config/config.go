// Package config owns a2v's on-disk state: proxies, model catalogue,
// aliases and the per-directory model pin.
//
// Layout (compatible with veicrun's files so an existing setup carries over):
//
//	~/.config/a2v/config.json    global config
//	./.a2v.json                  per-directory pin ({"model":"...","proxy":"..."})
//	~/.veicrun/veicrun.json      legacy source, imported once
package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const Version = 1

// Model is one entry of the launch menu.
type Model struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
	Desc string `json:"desc,omitempty"`
}

// Proxy is one upstream relay. Auth is "x-api-key" for Anthropic-style
// endpoints and empty for bearer.
type Proxy struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	URL  string `json:"url"`
	Key  string `json:"key,omitempty"`
	Auth string `json:"auth,omitempty"`

	// Models is the catalogue this relay's key is actually authorised for.
	// It is what the menu shows; entries outside it are history and stay
	// hidden unless the user asks for them.
	Models   []Model `json:"models,omitempty"`
	SyncedAt string  `json:"synced_at,omitempty"`
}

// Config is the global config file.
type Config struct {
	Version     int               `json:"version"`
	Model       string            `json:"model"`
	Flags       string            `json:"flags,omitempty"`
	Models      []Model           `json:"models"`
	Aliases     map[string]string `json:"aliases,omitempty"`
	Proxies     []Proxy           `json:"proxies"`
	ActiveProxy string            `json:"active_proxy"`
	Recent      []string          `json:"recent,omitempty"`
}

// Local is the per-directory pin.
type Local struct {
	Model string `json:"model,omitempty"`
	Proxy string `json:"proxy,omitempty"`
}

// Dir returns the directory holding a2v's files.
func Dir() string {
	if d := os.Getenv("A2V_CONFIG_DIR"); d != "" {
		return d
	}
	if d, err := os.UserConfigDir(); err == nil {
		return filepath.Join(d, "a2v")
	}
	return filepath.Join(os.Getenv("HOME"), ".config", "a2v")
}

// Path returns the global config path.
func Path() string { return filepath.Join(Dir(), "config.json") }

// LegacyPath is veicrun's config, imported on first run.
func LegacyPath() string {
	return filepath.Join(os.Getenv("HOME"), ".veicrun", "veicrun.json")
}

// Load reads the config, importing veicrun's file the first time.
// It always returns a usable Config, even when the file is missing.
func Load() (*Config, error) {
	path := Path()
	b, err := os.ReadFile(path)
	if err == nil {
		var c Config
		if err := json.Unmarshal(b, &c); err != nil {
			return nil, fmt.Errorf("解析 %s 失败: %w", path, err)
		}
		c.normalize()
		return &c, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if c, ok := importLegacy(); ok {
		return c, c.Save()
	}
	c := &Config{Version: Version, Aliases: map[string]string{}}
	return c, nil
}

// importLegacy copies ~/.veicrun/veicrun.json when it exists.
func importLegacy() (*Config, bool) {
	b, err := os.ReadFile(LegacyPath())
	if err != nil {
		return nil, false
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, false
	}
	c.Version = Version
	c.normalize()
	return &c, true
}

// Migrated reports whether the config came from veicrun.
func (c *Config) Migrated() bool {
	_, err := os.Stat(LegacyPath())
	if err != nil {
		return false
	}
	// Only meaningful when a2v's own file did not exist a moment ago.
	_, own := os.Stat(Path())
	return own != nil
}

func (c *Config) normalize() {
	if c.Version == 0 {
		c.Version = Version
	}
	if c.Aliases == nil {
		c.Aliases = map[string]string{}
	}
	if c.Models == nil {
		c.Models = []Model{}
	}
	if c.Proxies == nil {
		c.Proxies = []Proxy{}
	}
	for i := range c.Proxies {
		c.Proxies[i].URL = strings.TrimRight(strings.TrimSpace(c.Proxies[i].URL), "/")
	}
}

// Save writes the config atomically with 0600 (it holds API keys).
func (c *Config) Save() error {
	c.normalize()
	if err := os.MkdirAll(filepath.Dir(Path()), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	tmp := Path() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, Path())
}

// Proxy returns the active proxy, or nil when none is configured.
func (c *Config) Proxy() *Proxy {
	for i := range c.Proxies {
		if c.Proxies[i].ID == c.ActiveProxy {
			return &c.Proxies[i]
		}
	}
	return nil
}

// ProxyByID finds a proxy by id or (case-insensitive) name.
func (c *Config) ProxyByID(id string) *Proxy {
	for i := range c.Proxies {
		if c.Proxies[i].ID == id {
			return &c.Proxies[i]
		}
	}
	for i := range c.Proxies {
		if strings.EqualFold(c.Proxies[i].Name, id) {
			return &c.Proxies[i]
		}
	}
	return nil
}

// SetModels records the catalogue the current key is authorised for.
func (p *Proxy) SetModels(list []Model) {
	p.Models = list
	p.SyncedAt = time.Now().Format("2006-01-02 15:04")
}

// Authorised reports whether id is in this proxy's synced catalogue.
// An empty catalogue means "never synced", so nothing is filtered out.
func (p *Proxy) Authorised(id string) bool {
	if len(p.Models) == 0 {
		return true
	}
	for _, m := range p.Models {
		if m.ID == id {
			return true
		}
	}
	return false
}

// UseProxy switches the active proxy.
func (c *Config) UseProxy(id string) error {
	if c.ProxyByID(id) == nil {
		return fmt.Errorf("没有这个代理: %s", id)
	}
	c.ActiveProxy = c.ProxyByID(id).ID
	return nil
}

// AddProxy appends a proxy, deriving a unique id from the name.
func (c *Config) AddProxy(p Proxy) (*Proxy, error) {
	p.Name = strings.TrimSpace(p.Name)
	p.URL = strings.TrimRight(strings.TrimSpace(p.URL), "/")
	if p.Name == "" {
		return nil, errors.New("代理名不能为空")
	}
	if p.URL == "" {
		return nil, errors.New("代理 URL 不能为空")
	}
	id := slug(p.Name)
	if id == "" {
		id = fmt.Sprintf("p%d", len(c.Proxies)+1)
	}
	base := id
	for n := 2; c.ProxyByID(id) != nil; n++ {
		id = fmt.Sprintf("%s-%d", base, n)
	}
	p.ID = id
	c.Proxies = append(c.Proxies, p)
	return &c.Proxies[len(c.Proxies)-1], nil
}

// RemoveProxy deletes a proxy and clears the active pointer when needed.
func (c *Config) RemoveProxy(id string) bool {
	for i := range c.Proxies {
		if c.Proxies[i].ID == id {
			c.Proxies = append(c.Proxies[:i], c.Proxies[i+1:]...)
			if c.ActiveProxy == id {
				c.ActiveProxy = ""
			}
			return true
		}
	}
	return false
}

func slug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	dash := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		default:
			if !dash && b.Len() > 0 {
				b.WriteByte('-')
				dash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

// Resolve turns an alias, unique model-name fragment or raw id into a model id.
func (c *Config) Resolve(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return c.Model
	}
	if v, ok := c.Aliases[s]; ok {
		return v
	}
	for _, m := range c.Models {
		if strings.EqualFold(m.ID, s) {
			return m.ID
		}
	}
	for _, m := range c.Models {
		if strings.EqualFold(m.Name, s) {
			return m.ID
		}
	}
	return s
}

// AliasOf returns the alias pointing at a model id, if any.
func (c *Config) AliasOf(id string) string {
	names := make([]string, 0, len(c.Aliases))
	for k := range c.Aliases {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		if c.Aliases[k] == id {
			return k
		}
	}
	return ""
}

// Touch records a model as most recently used.
func (c *Config) Touch(id string) {
	out := []string{id}
	for _, r := range c.Recent {
		if r != id && len(out) < 12 {
			out = append(out, r)
		}
	}
	c.Recent = out
}

// LocalPath walks up from dir looking for a per-directory pin.
func LocalPath(dir string) (string, *Local) {
	dir, _ = filepath.Abs(dir)
	home, _ := os.UserHomeDir()
	for i := 0; i < 8 && dir != "/" && dir != ""; i++ {
		for _, name := range []string{".a2v.json", ".veicrun.json"} {
			p := filepath.Join(dir, name)
			if b, err := os.ReadFile(p); err == nil {
				var l Local
				if json.Unmarshal(b, &l) == nil && (l.Model != "" || l.Proxy != "") {
					return p, &l
				}
			}
		}
		if home != "" && dir == home {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", nil
}

// MergeHistory appends models never seen before to the history catalogue.
func MergeHistory(old, fresh []Model) []Model {
	seen := make(map[string]bool, len(old))
	out := append([]Model{}, old...)
	for _, m := range old {
		seen[m.ID] = true
	}
	for _, m := range fresh {
		if seen[m.ID] {
			continue
		}
		seen[m.ID] = true
		out = append(out, m)
	}
	return out
}

// PruneTo drops every catalogue entry not present in keep.
func PruneTo(catalogue, keep []Model) []Model {
	ok := make(map[string]bool, len(keep))
	for _, m := range keep {
		ok[m.ID] = true
	}
	out := catalogue[:0]
	for _, m := range catalogue {
		if ok[m.ID] {
			out = append(out, m)
		}
	}
	return out
}

// KeyFingerprint is a short, non-reversible tag for a key, used in diagnostics.
func KeyFingerprint(k string) string {
	if k == "" {
		return "-"
	}
	sum := sha256.Sum256([]byte(k))
	return hex.EncodeToString(sum[:])[:8]
}

// MaskKey renders sk-abcdef…wxyz for display.
func MaskKey(k string) string {
	if k == "" {
		return "(未设置)"
	}
	if len(k) <= 12 {
		return k[:3] + "…"
	}
	return k[:6] + "…" + k[len(k)-4:]
}

// WriteLocal persists a per-directory pin (0600, it mirrors the global file).
func WriteLocal(path string, l *Local) error {
	b, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o600)
}
