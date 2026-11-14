package tui

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/Shippuurl/a2v/internal/config"
	"github.com/Shippuurl/a2v/internal/remote"
	"github.com/Shippuurl/a2v/internal/version"
)

// chrome is the number of rows the frame (header + footer) occupies.
func (m *App) chrome() int {
	return 9
}

func (m *App) phase() float64 {
	return float64(m.frame) * 0.004
}

func (m *App) width() int {
	w := m.w
	if w <= 0 {
		w = 80
	}
	if w > 100 {
		w = 100
	}
	if w < 24 {
		w = 24
	}
	return w
}

// viewSplash is the launch screen: the brand banner, a metallic sweep, and a
// one-line hint. Any key (or 900ms) moves on.
func (m *App) viewSplash() string {
	w := m.width()
	art := renderArt(compactBanner, m.phase())
	var b strings.Builder
	pad := (m.h - len(art) - 6) / 2
	if pad > 0 {
		b.WriteString(strings.Repeat("\n", pad))
	}
	for _, line := range art {
		b.WriteString(center(line, w) + "\n")
	}
	b.WriteString("\n")
	b.WriteString(center(gradient("a2vec · codex launcher", m.phase()*0.7), w) + "\n")
	b.WriteString(center(style(faint).Render("按任意键继续  ·  "+version.Version), w) + "\n")
	return b.String()
}

func (m *App) View() string {
	if m.splash {
		return m.viewSplash()
	}
	if m.w > 0 && (m.w < 34 || m.h < 10) {
		return "\n  " + style(warn).Render("窗口太小") + style(faint).Render(" 请放大终端 (需要 ≥ 34×10)") + "\n"
	}
	cw := m.width()
	var b strings.Builder

	b.WriteString("\n")
	b.WriteString("  " + logo(m.phase(), "a2vec · codex launcher") + "\n")
	b.WriteString("  " + shimmer(cw-2, m.phase()*0.6, faint) + "\n")
	b.WriteString("  " + m.metaLine(cw-2) + "\n")
	b.WriteString("\n")

	switch m.screen {
	case scrProxies:
		b.WriteString(m.viewProxies(cw - 2))
	case scrSessions:
		b.WriteString(m.viewSessions(cw - 2))
	case scrKey:
		b.WriteString(m.viewKey(cw - 2))
	case scrHelp:
		b.WriteString(m.viewHelp(cw - 2))
	case scrAddProxy:
		b.WriteString(m.viewAddProxy(cw - 2))
	default:
		b.WriteString(m.viewModels(cw - 2))
	}

	b.WriteString("\n")
	if m.confirm != nil {
		b.WriteString("  " + style(warn).Bold(true).Render("⚠ "+m.confirm.prompt) +
			style(faint).Render("   y 确认 · n 取消") + "\n")
	}
	if s := m.statusLine(cw - 2); s != "" {
		b.WriteString("  " + s + "\n")
	}
	b.WriteString("  " + m.footer(cw-2) + "\n")
	return b.String()
}

// metaLine shows the three things that decide a launch: proxy, model, dir.
func (m *App) metaLine(w int) string {
	p := "—"
	if ap := m.cfg.Proxy(); ap != nil {
		p = ap.Name
		if ap.Key == "" {
			p += " (无 key)"
		}
	}
	model := m.cfg.Model
	if m.pin != nil && m.pin.Model != "" {
		model = m.pin.Model + " · pinned"
	}
	if model == "" {
		model = "未选"
	}
	dir := shortenPath(m.cwd)
	if lipgloss.Width(dir) > 30 {
		dir = "…" + dir[len(dir)-29:]
	}
	return pill("proxy", p, accent) + style(faint).Render("   ") +
		pill("model", model, iris) + style(faint).Render("   ") +
		pill("dir", dir, subtle)
}

func (m *App) viewModels(w int) string {
	if len(m.cfg.Models) == 0 {
		return m.emptyState(w)
	}
	var b strings.Builder
	if m.searching || m.search.Value() != "" {
		b.WriteString("  " + m.search.View() + "\n\n")
	}
	if len(m.filtered) == 0 {
		b.WriteString("  " + style(faint).Render("没有匹配的模型") + "\n")
		return b.String()
	}

	n := m.pageSize()
	end := m.offset + n
	if end > len(m.filtered) {
		end = len(m.filtered)
	}
	aliasW := 0
	for _, mo := range m.filtered[m.offset:end] {
		if a := m.cfg.AliasOf(mo.ID); a != "" && len(a) > aliasW {
			aliasW = len(a)
		}
	}
	if aliasW > 0 {
		aliasW += 2
	}
	for i := m.offset; i < end; i++ {
		b.WriteString(m.modelRow(i, m.filtered[i], w, aliasW) + "\n")
	}
	if len(m.filtered) > n {
		b.WriteString("  " + style(faint).Render(fmt.Sprintf("%d–%d / %d", m.offset+1, end, len(m.filtered))) + "\n")
	}
	return b.String()
}

func (m *App) modelRow(i int, mo config.Model, w, aliasW int) string {
	selected := i == m.cursor
	cursor := "   "
	if selected {
		cursor = gradient("▌", m.phase()) + "  "
	}
	alias := m.cfg.AliasOf(mo.ID)
	name := displayName(mo)
	stale := !m.authorised[mo.ID]
	nameStyle := lipgloss.NewStyle().Foreground(ink)
	if stale {
		nameStyle = lipgloss.NewStyle().Foreground(faint)
	}
	if selected {
		nameStyle = nameStyle.Bold(true).Foreground(lipgloss.Color("#ffffff"))
	}
	desc := ""
	if mo.Desc != "" && !strings.Contains(strings.ToLower(name), strings.ToLower(mo.Desc)) {
		desc = "  " + style(faint).Render(mo.Desc)
	}
	if stale {
		desc += "  " + style(warn).Render("未授权")
	}
	if m.dupNames[name] {
		short := mo.ID
		if i := strings.LastIndex(short, "/"); i >= 0 {
			short = short[i+1:]
		}
		desc += "  " + style(faint).Render(trunc(short, 26))
	}
	right := ""
	if alias != "" {
		right = badge(alias)
	}
	left := cursor + style(faint).Render(fmt.Sprintf("%2d", i+1)) + "  " + nameStyle.Render(name) + desc
	gap := w - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		left = trunc(left, w-lipgloss.Width(right)-1)
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

func (m *App) emptyState(w int) string {
	var b strings.Builder
	b.WriteString("  " + style(subtle).Render("还没有模型目录。") + "\n\n")
	b.WriteString("  " + style(faint).Render("先按 ") + style(accent).Render("p") + style(faint).Render(" 加一个中转（名字 / URL / Key），再按 ") +
		style(accent).Render("s") + style(faint).Render(" 拉模型。") + "\n")
	b.WriteString("  " + style(faint).Render("也可以直接编辑 ") + style(subtle).Render("~/.config/a2v/config.json") + "\n")
	return b.String()
}

func (m *App) viewProxies(w int) string {
	if len(m.cfg.Proxies) == 0 {
		return "  " + style(subtle).Render("还没有代理。按 a 添加一个中转。") + "\n"
	}
	var b strings.Builder
	for i, p := range m.cfg.Proxies {
		sel := i == m.pCursor
		cur := "   "
		if sel {
			cur = gradient("▌", m.phase()) + "  "
		}
		dot := "○"
		dotStyle := style(faint)
		if p.ID == m.cfg.ActiveProxy {
			dot = "●"
			dotStyle = style(good)
		}
		name := lipgloss.NewStyle().Foreground(ink)
		if sel {
			name = name.Bold(true)
		}
		key := config.MaskKey(p.Key)
		label := p.Name
		if p.Builtin {
			label += " ·内置"
		}
		line := cur + dotStyle.Render(dot) + "  " + name.Render(fill(trunc(label, 15), 16)) +
			style(faint).Render(fill(trunc(p.URL, 34), 35))
		right := style(faint).Render(key)
		gap := w - lipgloss.Width(line) - lipgloss.Width(right)
		if gap < 1 {
			line = trunc(line, w-lipgloss.Width(right)-1)
			gap = 1
		}
		b.WriteString(line + strings.Repeat(" ", gap) + right + "\n")
	}
	return b.String()
}

func (m *App) viewSessions(w int) string {
	if m.loading {
		return "  " + m.spin.View() + style(faint).Render(" 正在读取 ~/.codex/sessions …") + "\n"
	}
	if m.loadErr != nil {
		return "  " + style(bad).Render("读取失败: "+m.loadErr.Error()) + "\n"
	}
	list := m.visibleSessions()
	subCount := 0
	for _, s := range m.sessions {
		if s.Subagent {
			subCount++
		}
	}
	hidden := subCount
	if len(list) == 0 {
		if hidden > 0 {
			return "  " + style(faint).Render(fmt.Sprintf("只有 %d 条子代理会话（a 显示）", hidden)) + "\n"
		}
		return "  " + style(faint).Render("没有可恢复的会话") + "\n"
	}

	var b strings.Builder
	if hidden > 0 && !m.showSubagents {
		b.WriteString("  " + style(faint).Render(fmt.Sprintf("%d 条子代理会话已隐藏（a 显示）", hidden)) + "\n\n")
	} else if m.showSubagents {
		b.WriteString("  " + style(faint).Render(fmt.Sprintf("全部 %d 条（含 %d 条子代理）", len(list), subCount)) + "\n\n")
	}

	n := m.pageSize()
	end := m.sOffset + n
	if end > len(list) {
		end = len(list)
	}
	if m.sOffset > len(list) {
		m.sOffset = 0
	}
	for i := m.sOffset; i < end; i++ {
		s := list[i]
		sel := i == m.sCursor
		cur := "   "
		if sel {
			cur = gradient("▌", m.phase()) + "  "
		}
		model := s.Model
		if len(model) > 18 {
			model = model[:17] + "…"
		}
		title := s.Title
		tag := ""
		if s.Subagent {
			tag = style(faint).Render("  子代理")
		}
		// 3 cursor + 11 time + 2 gap + 19 model + tag + 1 margin
		titleW := w - (3 + 11 + 2 + 19 + lipgloss.Width(tag) + 1)
		if titleW < 10 {
			titleW = 10
		}
		line := cur + style(faint).Render(s.Time.Format("01-02 15:04")) + "  " +
			style(iris).Render(fill(model, 19)) + style(inkOrDim(sel)).Render(trunc(title, titleW)) + tag
		b.WriteString(line + "\n")
	}
	return b.String()
}

// displayName prefers the catalogue name, but upgrades names that are only a
// re-cased id ("Gpt 6 Astra") to the brand-aware rendering ("GPT 6 Astra").
func displayName(mo config.Model) string {
	if mo.Name == "" {
		return remote.NameOf(mo.ID)
	}
	if mo.Name == remote.NaiveTitle(mo.ID) {
		return remote.NameOf(mo.ID)
	}
	return mo.Name
}

func inkOrDim(sel bool) lipgloss.TerminalColor {
	if sel {
		return lipgloss.Color("#ffffff")
	}
	return subtle
}

func (m *App) viewKey(w int) string {
	p := m.cfg.Proxy()
	name := "（没有代理）"
	cur := "—"
	if p != nil {
		name = p.Name + "  " + p.URL
		if m.cfg.EnvKey(p) != "" {
			cur = "来自环境变量（不落盘）"
		} else {
			cur = config.MaskKey(p.Key)
		}
	}
	var b strings.Builder
	title := "为当前代理设置 API Key"
	if p != nil && p.Builtin {
		title = "内置中转 " + p.URL + "，粘贴你自己的 API Key 即可"
	}
	b.WriteString("  " + style(subtle).Render(title) + "\n\n")
	b.WriteString("  " + style(faint).Render("代理   ") + style(ink).Render(name) + "\n")
	b.WriteString("  " + style(faint).Render("当前   ") + style(subtle).Render(cur) + "\n\n")
	b.WriteString("  " + m.keyInput.View() + "\n\n")
	b.WriteString("  " + style(faint).Render("回车保存并写入 ~/.codex/auth.json；Esc 取消") + "\n")
	return b.String()
}

func (m *App) viewAddProxy(w int) string {
	var b strings.Builder
	b.WriteString("  " + style(subtle).Render("添加中转代理") + "\n\n")
	for i := range m.form {
		mark := " "
		if i == m.formFocus {
			mark = gradient("▸", m.phase())
		}
		b.WriteString("  " + mark + " " + m.form[i].View() + "\n")
	}
	b.WriteString("\n  " + style(faint).Render("Tab 下一个字段 · 回车提交 · Esc 取消") + "\n")
	return b.String()
}

func (m *App) viewHelp(w int) string {
	rows := [][2]string{
		{"↑ ↓ / j k", "移动选择"},
		{"回车", "用当前模型启动 codex"},
		{"/", "按关键词过滤模型"},
		{"p", "切换 / 添加中转代理"},
		{"ctrl+k", "设置当前代理的 API Key"},
		{"r", "恢复最近的 codex 会话"},
		{"s", "从当前代理重新拉取模型列表（按 key 授权）"},
		{"t", "显示 / 折叠未授权的历史模型"},
		{"d", "删除选中的模型 / 中转（y 确认）"},
		{"?", "打开 / 关闭这个帮助"},
		{"q", "退出"},
	}
	var b strings.Builder
	b.WriteString("  " + style(subtle).Render("快捷键") + "\n\n")
	for _, r := range rows {
		b.WriteString("  " + style(accent).Render(fill(r[0], 14)) + style(ink).Render(r[1]) + "\n")
	}
	b.WriteString("\n  " + style(faint).Render("配置 ") + style(subtle).Render(config.Path()) + "\n")
	b.WriteString("  " + style(faint).Render("版本 ") + style(subtle).Render(version.Version) + "\n")
	return b.String()
}

func (m *App) statusLine(w int) string {
	if m.status == "" {
		return ""
	}
	if !m.statusErr && !strings.HasPrefix(m.status, "dry-run:") && time.Since(m.statusAt) > 6*time.Second {
		return ""
	}
	icon := style(good).Render("✓ ")
	c := subtle
	if m.statusErr {
		icon = style(bad).Render("✗ ")
		c = bad
	}
	return icon + style(c).Render(trunc(m.status, w-4))
}

func (m *App) footer(w int) string {
	var parts []string
	switch m.screen {
	case scrModels:
		parts = []string{hint("↑↓", "选择"), hint("⏎", "启动"), hint("/", "筛选"), hint("d", "删除"), hint("p", "代理"), hint("r", "会话"), hint("?", "帮助"), hint("q", "退出")}
		if len(m.stale) > 0 {
			label := fmt.Sprintf("历史 %d", len(m.stale))
			if m.showStale {
				label = "折叠历史"
			}
			parts = append([]string{hint("t", label)}, parts...)
		}
	case scrProxies:
		parts = []string{hint("↑↓", "选择"), hint("⏎", "切换"), hint("a", "添加"), hint("d", "删除"), hint("esc", "返回")}
	case scrSessions:
		parts = []string{hint("↑↓", "选择"), hint("⏎", "恢复"), hint("a", "子代理"), hint("esc", "返回")}
	case scrKey:
		parts = []string{hint("⏎", "保存"), hint("esc", "取消")}
	case scrAddProxy:
		parts = []string{hint("tab", "下一个"), hint("⏎", "提交"), hint("esc", "取消")}
	case scrHelp:
		parts = []string{hint("?", "关闭"), hint("q", "退出")}
	}
	sep := style(faint).Render(" · ")
	full := strings.Join(parts, sep)
	ver := style(faint).Render("   " + version.Version)
	if lipgloss.Width(full)+lipgloss.Width(ver) <= w {
		return full + ver
	}
	for len(parts) > 1 && lipgloss.Width(full) > w {
		parts = parts[:len(parts)-1]
		full = strings.Join(parts, sep)
	}
	return full
}

func shortenPath(p string) string {
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(p, home) {
		return "~" + strings.TrimPrefix(p, home)
	}
	return p
}
