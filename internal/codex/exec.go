package codex

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
)

func envOrEmpty(k string) string { return os.Getenv(k) }

// command builds an exec.Cmd, wrapping .cmd/.bat shims the way Windows
// expects (npm installs codex as a .cmd shim, which CreateProcess cannot
// start directly).
func command(name string, args ...string) *exec.Cmd {
	if needsCmdShim(runtime.GOOS, name) {
		return exec.Command("cmd", append([]string{"/c", name}, args...)...)
	}
	return exec.Command(name, args...)
}

// needsCmdShim reports whether name must be launched through cmd.exe. npm on
// Windows installs codex as a .cmd shim, which CreateProcess cannot run.
func needsCmdShim(goos, name string) bool {
	if goos != "windows" {
		return false
	}
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".cmd") || strings.HasSuffix(lower, ".bat")
}

// Version returns `codex --version`, or "" when it cannot be determined.
func Version() string {
	bin, err := Bin()
	if err != nil {
		return ""
	}
	out, err := command(bin, "--version").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// windowsCandidates lists where codex lands on a Windows box.
func windowsCandidates() []string {
	var out []string
	if v := envOrEmpty("APPDATA"); v != "" {
		out = append(out, v+`\npm\codex.cmd`)
	}
	if v := envOrEmpty("LOCALAPPDATA"); v != "" {
		out = append(out, v+`\Programs\codex\codex.exe`)
		out = append(out, v+`\Microsoft\WindowsApps\codex.exe`)
	}
	return out
}
