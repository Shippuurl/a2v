package codex

import (
	"strings"
	"sync"
	"testing"
)

// withHelp fakes the installed codex's help output for one test.
func withHelp(t *testing.T, help string) {
	t.Helper()
	helpOnce = sync.Once{}
	helpBody = ""
	helpLook = func() string { return help }
	t.Cleanup(func() {
		helpOnce = sync.Once{}
		helpBody = ""
		helpLook = nil
	})
}

const newHelp = `
  -s, --sandbox <SANDBOX_MODE>
      --dangerously-bypass-approvals-and-sandbox
      --full-auto
  resume  Resume a previous interactive session
`

const oldHelp = `
  -s, --sandbox <SANDBOX_MODE>
      --dangerously-skip-permissions
      --full-auto
  resume  Resume a previous interactive session
`

func TestResolveFlagsForward(t *testing.T) {
	withHelp(t, newHelp)
	got := resolveFlags([]string{"--dangerously-skip-permissions"})
	if got[0] != "--dangerously-bypass-approvals-and-sandbox" {
		t.Fatalf("新 codex 应换成新名字，得到 %v", got)
	}
}

func TestResolveFlagsBackward(t *testing.T) {
	withHelp(t, oldHelp)
	got := resolveFlags([]string{"--dangerously-bypass-approvals-and-sandbox"})
	if got[0] != "--dangerously-skip-permissions" {
		t.Fatalf("老 codex 应退回旧名字，得到 %v", got)
	}
}

func TestResolveFlagsKeepsUnknown(t *testing.T) {
	withHelp(t, newHelp)
	got := resolveFlags([]string{"--made-up-flag", "--full-auto"})
	if got[0] != "--made-up-flag" || got[1] != "--full-auto" {
		t.Fatalf("未知参数不该被改写，得到 %v", got)
	}
	if bad := UnsupportedFlags("--made-up-flag --full-auto"); len(bad) != 1 || bad[0] != "--made-up-flag" {
		t.Fatalf("UnsupportedFlags 应只报未知参数，得到 %v", bad)
	}
}

func TestResumeNeedsSubcommand(t *testing.T) {
	withHelp(t, newHelp)
	spec := Spec{Model: "gpt-5.6-sol", ResumeID: "abc"}
	if args := strings.Join(spec.Args(), " "); strings.Contains(args, "resume") == false {
		t.Fatalf("支持 resume 时应带上 resume：%s", args)
	}

	withHelp(t, strings.ReplaceAll(newHelp, "resume  Resume a previous interactive session", ""))
	if args := strings.Join(spec.Args(), " "); strings.Contains(args, "resume") {
		t.Fatalf("不支持 resume 时应退化成新会话：%s", args)
	}
}

func TestNoHelpLeavesFlagsAlone(t *testing.T) {
	withHelp(t, "")
	got := resolveFlags([]string{"--dangerously-skip-permissions"})
	if got[0] != "--dangerously-skip-permissions" {
		t.Fatalf("探测不到 codex 时应原样传递，得到 %v", got)
	}
}
