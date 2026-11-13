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
func syncModels(cfg *config.Config, p *config.Proxy) tea.Cmd {
	return func() tea.Msg {
		if p == nil {
			return modelsMsg{err: errNoProxy}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		list, err := remote.Sync(ctx, cfg.WithKey(p))
		return modelsMsg{list: list, err: err}
	}
}
