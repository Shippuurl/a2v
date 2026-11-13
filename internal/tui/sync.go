package tui

import (
	"context"
	"errors"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Shippuurl/a2v/internal/config"
	"github.com/Shippuurl/a2v/internal/remote"
)

type modelsMsg struct {
	list []config.Model
	err  error
}

var errNoProxy = errors.New("还没有代理，先按 p 添加一个")

// syncModels pulls the catalogue from the active relay.
func syncModels(p *config.Proxy) tea.Cmd {
	return func() tea.Msg {
		if p == nil {
			return modelsMsg{err: errNoProxy}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		list, err := remote.Sync(ctx, p)
		return modelsMsg{list: list, err: err}
	}
}

// mergeModels keeps entries the relay no longer advertises, so switching
// proxies never empties the menu.
func mergeModels(fresh, old []config.Model) []config.Model {
	seen := make(map[string]bool, len(fresh))
	out := make([]config.Model, 0, len(fresh)+len(old))
	for _, m := range fresh {
		if seen[m.ID] {
			continue
		}
		seen[m.ID] = true
		out = append(out, m)
	}
	for _, m := range old {
		if seen[m.ID] {
			continue
		}
		seen[m.ID] = true
		out = append(out, m)
	}
	return out
}
