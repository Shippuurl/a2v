package tui

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

// Run starts the interactive app. It returns once codex exits or the user quits.
func Run(o Options) error {
	m := New(o)
	prog := tea.NewProgram(m, tea.WithAltScreen())
	final, err := prog.Run()
	if err != nil {
		return err
	}
	if a, ok := final.(*App); ok && o.DryRun && a.status != "" {
		fmt.Fprintln(os.Stderr, a.status)
	}
	return nil
}
