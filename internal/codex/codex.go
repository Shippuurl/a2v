// Package codex wraps the local codex CLI: how a2v builds its command line,
// which environment it injects, and where its sessions live.
package codex

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Shippuurl/a2v/internal/config"
)

// Spec describes one launch.
type Spec struct {
	Model    string
	Proxy    *config.Proxy
	ResumeID string
	Flags    string   // extra codex flags from the config file
	Extra    []string // explicit args from the user ("a2v run ... -- <args>")
	Dir      string   // working directory (empty = inherit)
}

// Bin locates the codex binary.
func Bin() (string, error) {
	if v := os.Getenv("A2V_CODEX_BIN"); v != "" {
		return v, nil
	}
	if p, err := exec.LookPath("codex"); err == nil {
		return p, nil
	}
	home, _ := os.UserHomeDir()
	for _, p := range []string{
		filepath.Join(home, ".local", "bin", "codex"),
		"/usr/local/bin/codex",
		"/opt/homebrew/bin/codex",
	} {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, nil
		}
	}
	return "", fmt.Errorf("找不到 codex 可执行文件（装一个：npm i -g @openai/codex，或用 A2V_CODEX_BIN 指定）")
}

// Args builds the codex argv for this spec.
func (s Spec) Args() []string {
	var a []string

	if p := s.Proxy; p != nil {
		base := strings.TrimRight(p.URL, "/") + "/v1"
		a = append(a,
			"-c", `model_provider="proxy"`,
			"-c", fmt.Sprintf("model_providers.proxy.name=%q", p.Name),
			"-c", fmt.Sprintf("model_providers.proxy.base_url=%q", base),
			"-c", `model_providers.proxy.env_key="CODEX_PROXY_API_KEY"`,
			"-c", `model_providers.proxy.wire_api="responses"`,
		)
	}

	a = append(a, splitFlags(s.Flags)...)

	if s.ResumeID != "" {
		a = append(a, "resume", "--model", s.Model, s.ResumeID)
	} else {
		a = append(a, "--model", s.Model)
	}
	a = append(a, s.Extra...)
	return a
}

// Env returns the extra environment for the child process.
func (s Spec) Env() []string {
	var env []string
	if p := s.Proxy; p != nil {
		base := strings.TrimRight(p.URL, "/") + "/v1"
		env = append(env,
			"OPENAI_API_KEY="+p.Key,
			"CODEX_PROXY_API_KEY="+p.Key,
			"FASTLLM_API_KEY="+p.Key,
			"OPENAI_BASE_URL="+base,
		)
	}
	return env
}

// Command renders the spec for display (never includes the key).
func (s Spec) Command() string {
	parts := []string{"codex"}
	parts = append(parts, s.Args()...)
	return strings.Join(parts, " ")
}

// splitFlags splits a flag string on spaces while honouring quotes.
func splitFlags(s string) []string {
	var out []string
	var cur strings.Builder
	var quote rune
	flush := func() {
		if cur.Len() > 0 {
			out = append(out, cur.String())
			cur.Reset()
		}
	}
	for _, r := range s {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote = r
		case r == ' ' || r == '\t':
			flush()
		default:
			cur.WriteRune(r)
		}
	}
	flush()
	return out
}

// Exec runs codex in the given spec, replacing a2v's process on unix.
func (s Spec) Exec() error {
	bin, err := Bin()
	if err != nil {
		return err
	}
	args := s.Args()
	if s.Dir != "" {
		if err := os.Chdir(s.Dir); err != nil {
			return err
		}
	}
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(), s.Env()...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Cmd returns the exec.Cmd for a spec, used by the TUI's ExecProcess.
func (s Spec) Cmd() (*exec.Cmd, error) {
	bin, err := Bin()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(bin, s.Args()...)
	cmd.Env = append(os.Environ(), s.Env()...)
	cmd.Dir = s.Dir
	return cmd, nil
}

// SyncKey mirrors a proxy key into codex's own config files, the way the
// original launcher did: base_url in config.toml, OPENAI_API_KEY in auth.json.
// Files are only edited when they already exist; nothing is created.
func SyncKey(cfg *config.Config, p *config.Proxy) error {
	home, _ := os.UserHomeDir()
	base := strings.TrimRight(p.URL, "/") + "/v1"

	tomlPath := filepath.Join(home, ".codex", "config.toml")
	if b, err := os.ReadFile(tomlPath); err == nil {
		re := regexp.MustCompile(`(?s)(\[model_providers\.proxy\][^\[]*?base_url = ")[^"]+(")`)
		out := re.ReplaceAllString(string(b), "${1}"+base+"${2}")
		if out != string(b) {
			if err := os.WriteFile(tomlPath, []byte(out), 0o644); err != nil {
				return err
			}
		}
	}

	authPath := filepath.Join(home, ".codex", "auth.json")
	if b, err := os.ReadFile(authPath); err == nil {
		re := regexp.MustCompile(`"OPENAI_API_KEY":\s*"[^"]*"`)
		out := re.ReplaceAllString(string(b), `"OPENAI_API_KEY": "`+p.Key+`"`)
		if out != string(b) {
			if err := os.WriteFile(authPath, []byte(out), 0o600); err != nil {
				return err
			}
		}
	}
	return nil
}
