package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/Shippuurl/a2v/internal/codex"
	"github.com/Shippuurl/a2v/internal/config"
)

type screen int

const (
	scrModels screen = iota
	scrProxies
	scrSessions
	scrKey
	scrHelp
	scrAddProxy
)

type tickMsg time.Time

type sessionsMsg struct {
	list []codex.Session
	err  error
}

type launchDoneMsg struct{ err error }

// Options configures a TUI run.
type Options struct {
	Config   *config.Config
	Dir      string
	LocalPin *config.Local
	LocalAt  string
	DryRun   bool
}

// App is the root bubbletea model.
type App struct {
	cfg     *config.Config
	cwd     string
	pin     *config.Local
	pinPath string
	dryRun  bool

	w, h int

	screen screen
	frame  int
	quit   bool

	// model list
	cursor    int
	offset    int
	search    textinput.Model
	searching bool
	filtered  []config.Model

	// proxies
	pCursor int

	// sessions
	sessions []codex.Session
	sCursor  int
	loading  bool
	spin     spinner.Model
	loadErr  error

	// key entry
	keyInput textinput.Model

	// add-proxy form
	form      [3]textinput.Model
	formFocus int

	status    string
	statusErr bool
	statusAt  time.Time

	dupNames map[string]bool
	centered bool

	base       []config.Model // what the menu is currently built from
	authorised map[string]bool
	stale      []config.Model // history the current key is not authorised for
	showStale  bool
}

// New builds the app.
func New(o Options) *App {
	m := &App{cfg: o.Config, cwd: o.Dir, pin: o.LocalPin, pinPath: o.LocalAt, dryRun: o.DryRun}

	m.search = textinput.New()
	m.search.Placeholder = "输入关键词筛选模型"
	m.search.CharLimit = 64
	m.search.Prompt = style(iris).Render("⌕ ")

	m.keyInput = textinput.New()
	m.keyInput.Placeholder = "粘贴新的 API Key，回车保存"
	m.keyInput.EchoMode = textinput.EchoPassword
	m.keyInput.EchoCharacter = '•'
	m.keyInput.CharLimit = 512
	m.keyInput.Width = 48
	m.keyInput.Prompt = style(iris).Render("key ")

	labels := []string{"名称", "URL", "Key"}
	for i := range m.form {
		f := textinput.New()
		f.Placeholder = labels[i]
		f.CharLimit = 512
		f.Width = 56
		f.Prompt = style(faint).Render(labels[i] + " › ")
		m.form[i] = f
	}
	m.form[2].EchoMode = textinput.EchoPassword
	m.form[2].EchoCharacter = '•'

	m.spin = spinner.New()
	m.spin.Spinner = spinner.MiniDot
	m.spin.Style = style(iris)

	m.refresh()
	// Start on the pinned / configured model.
	target := m.cfg.Model
	if m.pin != nil && m.pin.Model != "" {
		target = m.cfg.Resolve(m.pin.Model)
	}
	for i, mo := range m.filtered {
		if mo.ID == target {
			m.cursor = i
			break
		}
	}
	m.clampOffset()
	return m
}

func (m *App) Init() tea.Cmd {
	return tickCmd()
}

func tickCmd() tea.Cmd {
	return tea.Tick(45*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tickMsg:
		m.frame++
		return m, tickCmd()

	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		if !m.centered {
			m.offset = m.cursor - (m.h-m.chrome())/2
			m.centered = true
		}
		m.clampOffset()
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd

	case modelsMsg:
		m.loading = false
		if msg.err != nil {
			m.setStatus("拉取模型失败: "+msg.err.Error(), true)
			return m, nil
		}
		p := m.cfg.Proxy()
		if p == nil {
			m.setStatus(errNoProxy.Error(), true)
			return m, nil
		}
		// The synced list becomes what this key is authorised for; anything
		// else becomes history (kept, but folded away).
		m.cfg.Models = config.MergeHistory(m.cfg.Models, msg.list)
		p.SetModels(msg.list)
		if err := m.cfg.Save(); err != nil {
			m.setStatus("保存失败: "+err.Error(), true)
			return m, nil
		}
		m.refresh()
		m.setStatus(fmt.Sprintf("%s 授权 %d 个模型（历史 %d 个已折叠）", p.Name, len(msg.list), len(m.stale)), false)
		return m, nil

	case sessionsMsg:
		m.loading = false
		if msg.err != nil {
			m.loadErr = msg.err
			m.setStatus("读取会话失败: "+msg.err.Error(), true)
		} else {
			m.sessions = msg.list
			m.sCursor = 0
			if len(msg.list) == 0 {
				m.setStatus("~/.codex/sessions 里没有可恢复的会话", false)
			}
		}
		return m, nil

	case launchDoneMsg:
		m.quit = true
		if msg.err != nil {
			m.statusErr = true
			m.status = "codex 退出: " + msg.err.Error()
			return m, tea.Quit
		}
		return m, tea.Quit

	case tea.KeyMsg:
		return m.onKey(msg)
	}

	// Let the focused input handle the event.
	var cmd tea.Cmd
	switch m.screen {
	case scrKey:
		m.keyInput, cmd = m.keyInput.Update(msg)
	case scrAddProxy:
		m.form[m.formFocus], cmd = m.form[m.formFocus].Update(msg)
	case scrModels:
		if m.searching {
			m.search, cmd = m.search.Update(msg)
			m.applyFilter()
		}
	}
	return m, cmd
}

func (m *App) onKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := k.String()

	if s == "ctrl+c" {
		m.quit = true
		return m, tea.Quit
	}

	// Text-entry screens own most keys.
	switch m.screen {
	case scrKey:
		switch s {
		case "esc":
			m.screen = scrModels
			m.keyInput.Blur()
			return m, nil
		case "enter":
			return m.saveKey()
		}
		var cmd tea.Cmd
		m.keyInput, cmd = m.keyInput.Update(k)
		return m, cmd

	case scrAddProxy:
		switch s {
		case "esc":
			m.screen = scrProxies
			m.form[m.formFocus].Blur()
			return m, nil
		case "tab", "down":
			m.form[m.formFocus].Blur()
			m.formFocus = (m.formFocus + 1) % len(m.form)
			m.form[m.formFocus].Focus()
			return m, nil
		case "shift+tab", "up":
			m.form[m.formFocus].Blur()
			m.formFocus = (m.formFocus + len(m.form) - 1) % len(m.form)
			m.form[m.formFocus].Focus()
			return m, nil
		case "enter":
			if m.formFocus < len(m.form)-1 {
				m.form[m.formFocus].Blur()
				m.formFocus++
				m.form[m.formFocus].Focus()
				return m, nil
			}
			return m.submitProxy()
		}
		var cmd tea.Cmd
		m.form[m.formFocus], cmd = m.form[m.formFocus].Update(k)
		return m, cmd

	case scrModels:
		if m.searching {
			switch s {
			case "esc":
				m.searching = false
				m.search.Blur()
				m.search.SetValue("")
				m.applyFilter()
				return m, nil
			case "enter":
				m.searching = false
				m.search.Blur()
				return m, nil
			}
			var cmd tea.Cmd
			m.search, cmd = m.search.Update(k)
			m.applyFilter()
			return m, cmd
		}
	}

	switch s {
	case "q":
		if m.screen != scrModels {
			m.screen = scrModels
			return m, nil
		}
		m.quit = true
		return m, tea.Quit
	case "?":
		if m.screen == scrHelp {
			m.screen = scrModels
		} else {
			m.screen = scrHelp
		}
		return m, nil
	case "esc":
		if m.screen != scrModels {
			m.screen = scrModels
			return m, nil
		}
	}

	switch m.screen {
	case scrModels:
		return m.onModelsKey(s)
	case scrProxies:
		return m.onProxiesKey(s)
	case scrSessions:
		return m.onSessionsKey(s)
	}
	return m, nil
}

func (m *App) onModelsKey(s string) (tea.Model, tea.Cmd) {
	switch s {
	case "up", "k":
		m.move(-1)
	case "down", "j":
		m.move(1)
	case "pgup":
		m.move(-m.pageSize())
	case "pgdown":
		m.move(m.pageSize())
	case "home":
		m.cursor = 0
		m.clampOffset()
	case "end":
		m.cursor = len(m.filtered) - 1
		m.clampOffset()
	case "/":
		m.searching = true
		m.search.Focus()
		return m, textinput.Blink
	case "t":
		if len(m.stale) == 0 {
			m.setStatus("没有历史模型可显示", false)
			return m, nil
		}
		m.showStale = !m.showStale
		m.refresh()
		if m.showStale {
			m.setStatus(fmt.Sprintf("显示 %d 个历史模型（当前 key 未授权）", len(m.stale)), false)
		} else {
			m.setStatus(fmt.Sprintf("已折叠 %d 个未授权模型", len(m.stale)), false)
		}
		return m, nil
	case "s":
		m.loading = true
		return m, tea.Batch(m.spin.Tick, syncModels(m.cfg.Proxy()))
	case "p":
		m.screen = scrProxies
		m.pCursor = m.proxyIndex()
	case "r":
		m.screen = scrSessions
		m.loading = true
		return m, tea.Batch(m.spin.Tick, loadSessions())
	case "ctrl+k":
		m.screen = scrKey
		m.keyInput.SetValue("")
		m.keyInput.Focus()
		return m, textinput.Blink
	case "enter":
		return m.launch("")
	}
	return m, nil
}

func (m *App) onProxiesKey(s string) (tea.Model, tea.Cmd) {
	switch s {
	case "up", "k":
		if m.pCursor > 0 {
			m.pCursor--
		}
	case "down", "j":
		if m.pCursor < len(m.cfg.Proxies)-1 {
			m.pCursor++
		}
	case "a":
		m.screen = scrAddProxy
		m.formFocus = 0
		for i := range m.form {
			m.form[i].SetValue("")
			m.form[i].Blur()
		}
		m.form[0].Focus()
		return m, textinput.Blink
	case "enter":
		if m.pCursor >= 0 && m.pCursor < len(m.cfg.Proxies) {
			p := m.cfg.Proxies[m.pCursor]
			m.cfg.ActiveProxy = p.ID
			if err := m.cfg.Save(); err != nil {
				m.setStatus("保存失败: "+err.Error(), true)
			} else {
				m.setStatus("已切换到 "+p.Name, false)
			}
		}
	}
	return m, nil
}

func (m *App) onSessionsKey(s string) (tea.Model, tea.Cmd) {
	switch s {
	case "up", "k":
		if m.sCursor > 0 {
			m.sCursor--
		}
	case "down", "j":
		if m.sCursor < len(m.sessions)-1 {
			m.sCursor++
		}
	case "enter":
		if m.sCursor >= 0 && m.sCursor < len(m.sessions) {
			return m.launch(m.sessions[m.sCursor].ID)
		}
	}
	return m, nil
}

func loadSessions() tea.Cmd {
	return func() tea.Msg {
		list, err := codex.Recent(24)
		return sessionsMsg{list: list, err: err}
	}
}

// launch starts codex with the selected model (and optional resume id).
func (m *App) launch(resumeID string) (tea.Model, tea.Cmd) {
	if len(m.filtered) == 0 {
		return m, nil
	}
	model := m.filtered[m.cursor].ID

	m.cfg.Model = model
	m.cfg.Touch(model)
	if m.pin != nil && m.pin.Model != "" {
		m.pin.Model = model
		_ = config.WriteLocal(m.pinPath, m.pin)
	}
	if err := m.cfg.Save(); err != nil {
		m.setStatus("保存失败: "+err.Error(), true)
	}

	spec := codex.Spec{
		Model:    model,
		Proxy:    m.cfg.Proxy(),
		Flags:    m.cfg.Flags,
		ResumeID: resumeID,
		Dir:      m.cwd,
	}
	if m.dryRun {
		m.quit = true
		m.status = "dry-run: " + spec.Command()
		return m, tea.Quit
	}
	cmd, err := spec.Cmd()
	if err != nil {
		m.setStatus(err.Error(), true)
		return m, nil
	}
	return m, tea.ExecProcess(cmd, func(err error) tea.Msg { return launchDoneMsg{err} })
}

func (m *App) saveKey() (tea.Model, tea.Cmd) {
	k := strings.TrimSpace(m.keyInput.Value())
	if k == "" {
		m.screen = scrModels
		return m, nil
	}
	p := m.cfg.Proxy()
	if p == nil {
		m.setStatus("先添加一个代理（p → a）", true)
		m.screen = scrProxies
		return m, nil
	}
	p.Key = k
	if err := m.cfg.Save(); err != nil {
		m.setStatus("保存失败: "+err.Error(), true)
		return m, nil
	}
	if err := codex.SyncKey(m.cfg, p); err != nil {
		m.setStatus("Key 已存 a2v，但同步 codex 配置失败: "+err.Error(), true)
	} else {
		m.setStatus(p.Name+" 的 Key 已更新", false)
	}
	m.screen = scrModels
	m.keyInput.Blur()
	return m, nil
}

func (m *App) submitProxy() (tea.Model, tea.Cmd) {
	name := strings.TrimSpace(m.form[0].Value())
	url := strings.TrimSpace(m.form[1].Value())
	key := strings.TrimSpace(m.form[2].Value())
	if name == "" || url == "" {
		m.setStatus("名称和 URL 不能为空", true)
		return m, nil
	}
	p, err := m.cfg.AddProxy(config.Proxy{Name: name, URL: url, Key: key})
	if err != nil {
		m.setStatus(err.Error(), true)
		return m, nil
	}
	m.cfg.ActiveProxy = p.ID
	if err := m.cfg.Save(); err != nil {
		m.setStatus("保存失败: "+err.Error(), true)
		return m, nil
	}
	m.setStatus("已添加代理 "+p.Name, false)
	m.screen = scrProxies
	m.pCursor = len(m.cfg.Proxies) - 1
	for i := range m.form {
		m.form[i].Blur()
	}
	return m, nil
}

func (m *App) setStatus(s string, isErr bool) {
	m.status = s
	m.statusErr = isErr
	m.statusAt = time.Now()
}

func (m *App) proxyIndex() int {
	for i, p := range m.cfg.Proxies {
		if p.ID == m.cfg.ActiveProxy {
			return i
		}
	}
	return 0
}

func (m *App) pageSize() int {
	n := m.h - m.chrome()
	if n < 3 {
		return 3
	}
	return n
}

func (m *App) move(d int) {
	if len(m.filtered) == 0 {
		return
	}
	m.cursor += d
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor > len(m.filtered)-1 {
		m.cursor = len(m.filtered) - 1
	}
	m.clampOffset()
}

func (m *App) clampOffset() {
	n := m.pageSize()
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+n {
		m.offset = m.cursor - n + 1
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

func (m *App) applyFilter() {
	q := strings.ToLower(strings.TrimSpace(m.search.Value()))
	m.filtered = m.filtered[:0]
	for _, mo := range m.base {
		if q == "" || strings.Contains(strings.ToLower(mo.ID), q) ||
			strings.Contains(strings.ToLower(mo.Name), q) ||
			strings.Contains(strings.ToLower(mo.Desc), q) {
			m.filtered = append(m.filtered, mo)
		}
	}
	m.cursor = 0
	m.offset = 0
}

// refresh rebuilds the menu from what the active key is authorised for.
// Models the key cannot use are kept as history and folded away by default.
func (m *App) refresh() {
	keep := ""
	if m.cursor >= 0 && m.cursor < len(m.filtered) {
		keep = m.filtered[m.cursor].ID
	}

	m.authorised = map[string]bool{}
	m.stale = nil

	var src []config.Model
	if p := m.cfg.Proxy(); p != nil && len(p.Models) > 0 {
		for _, mo := range p.Models {
			m.authorised[mo.ID] = true
		}
		for _, mo := range m.cfg.Models {
			if !m.authorised[mo.ID] {
				m.stale = append(m.stale, mo)
			}
		}
		src = append(src, p.Models...)
		if m.showStale {
			src = append(src, m.stale...)
		}
	} else {
		src = append(src, m.cfg.Models...)
		for _, mo := range src {
			m.authorised[mo.ID] = true
		}
	}

	m.dupNames = map[string]bool{}
	count := map[string]int{}
	for _, mo := range src {
		count[displayName(mo)]++
	}
	for name, n := range count {
		if n > 1 {
			m.dupNames[name] = true
		}
	}

	m.base = src
	m.applyFilter()
	for i, mo := range m.filtered {
		if mo.ID == keep {
			m.cursor = i
			break
		}
	}
	m.clampOffset()
}
