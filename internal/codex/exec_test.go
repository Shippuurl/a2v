package codex

import "testing"

func TestNeedsCmdShim(t *testing.T) {
	cases := []struct {
		goos, name string
		want       bool
	}{
		{"windows", `C:\Users\x\AppData\Roaming\npm\codex.cmd`, true},
		{"windows", `C:\tools\codex.BAT`, true},
		{"windows", `C:\tools\codex.exe`, false},
		{"linux", "/usr/local/bin/codex.cmd", false},
	}
	for _, c := range cases {
		if got := needsCmdShim(c.goos, c.name); got != c.want {
			t.Errorf("needsCmdShim(%q,%q)=%v want %v", c.goos, c.name, got, c.want)
		}
	}
}
