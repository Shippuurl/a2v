// Package cli is a2v's command surface: one binary, a short menu of verbs,
// and a TUI when there is a terminal to draw on.
package cli

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/Shippuurl/a2v/internal/codex"
	"github.com/Shippuurl/a2v/internal/config"
	"github.com/Shippuurl/a2v/internal/remote"
	"github.com/Shippuurl/a2v/internal/tui"
	"github.com/Shippuurl/a2v/internal/version"
)

var dryRun bool

// Execute runs the CLI.
func Execute() error {
	return newRoot().Execute()
}

func newRoot() *cobra.Command {
	root := &cobra.Command{
		Use:           "a2v [model|alias]",
		Short:         "a2vec 的命令行启动器（codex + 中转代理）",
		Long:          "a2v 把 codex、多个中转代理、模型目录和会话恢复收进一个菜单。\n不带参数运行进入交互界面；带模型名或别名直接启动。",
		Version:       version.Version,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ArbitraryArgs,
		RunE:          runRoot,
	}
	root.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "只打印将要执行的 codex 命令，不启动")
	root.SetVersionTemplate("a2v {{.Version}}\n")
	root.CompletionOptions.DisableDefaultCmd = true

	root.AddCommand(
		newModelsCmd(),
		newRunCmd(),
		newProxyCmd(),
		newKeyCmd(),
		newSyncCmd(),
		newSessionsCmd(),
		newResumeCmd(),
		newPinCmd(),
		newDoctorCmd(),
		newUpdateCmd(),
		newVersionCmd(),
	)
	return root
}

// ---------------------------------------------------------------- root

func runRoot(_ *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	cwd, _ := os.Getwd()
	pinPath, pin := config.LocalPath(cwd)

	if len(args) > 0 {
		return launch(cfg, cfg.Resolve(args[0]), "", nil)
	}
	if !isTTY() {
		return fallbackMenu(cfg, pin)
	}
	return tui.Run(tui.Options{Config: cfg, Dir: cwd, LocalPin: pin, LocalAt: pinPath, DryRun: dryRun})
}

// launch hands the terminal to codex.
func launch(cfg *config.Config, model, resume string, extra []string) error {
	if model == "" {
		return fmt.Errorf("没有可选模型：先运行 a2v 配置代理，或 a2v sync 拉取模型")
	}
	spec := codex.Spec{
		Model:    model,
		Proxy:    cfg.Proxy(),
		Flags:    cfg.Flags,
		ResumeID: resume,
		Extra:    extra,
	}
	cfg.Model = model
	cfg.Touch(model)
	if err := cfg.Save(); err != nil {
		fmt.Fprintln(os.Stderr, "a2v: 保存偏好失败: "+err.Error())
	}
	if dryRun {
		fmt.Println(spec.Command())
		return nil
	}
	return spec.Exec()
}

// fallbackMenu is the no-TTY path: a plain numbered prompt.
func fallbackMenu(cfg *config.Config, pin *config.Local) error {
	if len(cfg.Models) == 0 {
		return fmt.Errorf("没有模型目录，先运行 a2v sync（或设置 A2V_CONFIG_DIR 指向已有配置）")
	}
	def := cfg.Model
	if pin != nil && pin.Model != "" {
		def = cfg.Resolve(pin.Model)
	}
	fmt.Println("a2v · 选择模型")
	if ap := cfg.Proxy(); ap != nil {
		fmt.Printf("代理: %s (%s)\n\n", ap.Name, ap.URL)
	}
	for i, m := range cfg.Models {
		mark := " "
		if m.ID == def {
			mark = "*"
		}
		alias := cfg.AliasOf(m.ID)
		if alias != "" {
			alias = " [" + alias + "]"
		}
		fmt.Printf("  %s %2d) %-26s %s%s\n", mark, i+1, m.ID, m.Desc, alias)
	}
	fmt.Printf("\n选择 [1-%d]（回车=默认 %s，q 退出）> ", len(cfg.Models), def)

	r := bufio.NewReader(os.Stdin)
	line, err := r.ReadString('\n')
	if err != nil && line == "" {
		return nil
	}
	line = strings.TrimSpace(line)
	switch {
	case line == "q" || line == "quit":
		return nil
	case line == "":
		return launch(cfg, def, "", nil)
	}
	if n, err := strconv.Atoi(line); err == nil {
		if n < 1 || n > len(cfg.Models) {
			return fmt.Errorf("序号超出范围")
		}
		return launch(cfg, cfg.Models[n-1].ID, "", nil)
	}
	return launch(cfg, cfg.Resolve(line), "", nil)
}

// ---------------------------------------------------------------- models

func newModelsCmd() *cobra.Command {
	var aliases bool
	cmd := &cobra.Command{
		Use:   "models",
		Short: "列出模型目录",
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if len(cfg.Models) == 0 {
				fmt.Println("(空) 运行 a2v sync 拉取")
				return nil
			}
			w := 0
			for _, m := range cfg.Models {
				if len(m.ID) > w {
					w = len(m.ID)
				}
			}
			ap := cfg.Proxy()
			for _, m := range cfg.Models {
				row := fmt.Sprintf("  %-*s  %-10s", w, m.ID, m.Desc)
				if a := cfg.AliasOf(m.ID); a != "" {
					row += " @" + a
				}
				if m.ID == cfg.Model {
					row += "  ← 当前"
				}
				if ap != nil && len(ap.Models) > 0 && !ap.Authorised(m.ID) {
					row += "  (未授权)"
				}
				fmt.Println(row)
			}
			if ap != nil {
				if len(ap.Models) > 0 {
					fmt.Printf("\n当前代理 %s：授权 %d 个（同步于 %s）\n", ap.Name, len(ap.Models), ap.SyncedAt)
				} else {
					fmt.Printf("\n当前代理 %s：尚未同步，运行 a2v sync\n", ap.Name)
				}
			}
			if aliases {
				fmt.Println()
				keys := make([]string, 0, len(cfg.Aliases))
				for k := range cfg.Aliases {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				for _, k := range keys {
					fmt.Printf("  @%s → %s\n", k, cfg.Aliases[k])
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&aliases, "aliases", false, "同时列出别名")
	cmd.AddCommand(newModelsRmCmd(), newModelsPruneCmd())
	return cmd
}

func newModelsRmCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rm <id|alias>...",
		Short: "从目录里删除模型（历史模型和授权模型都会删）",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			for _, arg := range args {
				id := cfg.Resolve(arg)
				if !dropModel(cfg, id) {
					fmt.Printf("  没有 %s\n", arg)
					continue
				}
				fmt.Printf("  已删除 %s\n", id)
			}
			if cfg.Model != "" {
				found := false
				for _, m := range cfg.Models {
					if m.ID == cfg.Model {
						found = true
					}
				}
				if !found {
					cfg.Model = ""
				}
			}
			return cfg.Save()
		},
	}
}

// dropModel removes id from the history catalogue and every proxy's list.
func dropModel(cfg *config.Config, id string) bool {
	found := false
	out := cfg.Models[:0]
	for _, m := range cfg.Models {
		if m.ID == id {
			found = true
			continue
		}
		out = append(out, m)
	}
	cfg.Models = out
	for i := range cfg.Proxies {
		if len(cfg.Proxies[i].Models) == 0 {
			continue
		}
		keep := cfg.Proxies[i].Models[:0]
		for _, m := range cfg.Proxies[i].Models {
			if m.ID != id {
				keep = append(keep, m)
			}
		}
		cfg.Proxies[i].Models = keep
	}
	return found
}

func newModelsPruneCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "prune",
		Short: "删掉当前 key 未授权的历史模型",
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			p := cfg.Proxy()
			if p == nil || len(p.Models) == 0 {
				return fmt.Errorf("当前代理还没有授权列表，先 a2v sync")
			}
			before := len(cfg.Models)
			cfg.Models = config.PruneTo(cfg.Models, p.Models)
			if err := cfg.Save(); err != nil {
				return err
			}
			fmt.Printf("删除历史模型 %d 个，剩余 %d 个\n", before-len(cfg.Models), len(cfg.Models))
			return nil
		},
	}
}

// ---------------------------------------------------------------- run

func newRunCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "run <model|alias> [-- codex 参数...]",
		Short: "用指定模型直接启动 codex",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			extra := args[1:]
			if len(extra) > 0 && extra[0] == "--" {
				extra = extra[1:]
			}
			return launch(cfg, cfg.Resolve(args[0]), "", extra)
		},
	}
}

// ---------------------------------------------------------------- proxy

func newProxyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "proxy",
		Short: "管理中转代理（list/use/add/rm）",
		RunE: func(_ *cobra.Command, _ []string) error {
			return listProxies()
		},
	}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "list",
			Short: "列出代理",
			RunE:  func(*cobra.Command, []string) error { return listProxies() },
		},
		&cobra.Command{
			Use:   "use <id|name>",
			Short: "切换当前代理",
			Args:  cobra.ExactArgs(1),
			RunE: func(_ *cobra.Command, args []string) error {
				cfg, err := config.Load()
				if err != nil {
					return err
				}
				if err := cfg.UseProxy(args[0]); err != nil {
					return err
				}
				if err := cfg.Save(); err != nil {
					return err
				}
				fmt.Printf("已切换到 %s (%s)\n", cfg.Proxy().Name, cfg.Proxy().URL)
				return nil
			},
		},
		&cobra.Command{
			Use:   "add <name> <url> [key]",
			Short: "添加代理并设为当前",
			Args:  cobra.RangeArgs(2, 3),
			RunE: func(_ *cobra.Command, args []string) error {
				cfg, err := config.Load()
				if err != nil {
					return err
				}
				p := config.Proxy{Name: args[0], URL: args[1]}
				if len(args) > 2 {
					p.Key = args[2]
				}
				added, err := cfg.AddProxy(p)
				if err != nil {
					return err
				}
				cfg.ActiveProxy = added.ID
				if err := cfg.Save(); err != nil {
					return err
				}
				if added.Key != "" {
					if err := codex.SyncKey(cfg, added); err != nil {
						fmt.Fprintln(os.Stderr, "a2v: 同步 codex 配置失败: "+err.Error())
					}
				}
				fmt.Printf("已添加 %s (%s) 并设为当前\n", added.Name, added.URL)
				return nil
			},
		},
		&cobra.Command{
			Use:   "rm <id|name>",
			Short: "删除代理",
			Args:  cobra.ExactArgs(1),
			RunE: func(_ *cobra.Command, args []string) error {
				cfg, err := config.Load()
				if err != nil {
					return err
				}
				p := cfg.ProxyByID(args[0])
				if p == nil {
					return fmt.Errorf("没有这个代理: %s", args[0])
				}
				cfg.RemoveProxy(p.ID)
				if err := cfg.Save(); err != nil {
					return err
				}
				fmt.Printf("已删除 %s\n", p.Name)
				return nil
			},
		},
	)
	return cmd
}

func listProxies() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if len(cfg.Proxies) == 0 {
		fmt.Println("(空) 用 a2v proxy add <name> <url> [key] 添加")
		return nil
	}
	for _, p := range cfg.Proxies {
		mark := " "
		if p.ID == cfg.ActiveProxy {
			mark = "●"
		}
		name := p.Name
		if p.Builtin {
			name += "·内置"
		}
		fmt.Printf("  %s %-16s %-42s %s\n", mark, name, p.URL, config.MaskKey(p.Key))
	}
	return nil
}

// ---------------------------------------------------------------- key

func newKeyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "key [value]",
		Short: "设置当前代理的 API Key（不传则从 stdin 读）",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			p := cfg.Proxy()
			if p == nil {
				return fmt.Errorf("还没有代理，先 a2v proxy add")
			}
			var k string
			if len(args) == 1 {
				k = strings.TrimSpace(args[0])
			} else {
				b, err := io.ReadAll(os.Stdin)
				if err != nil {
					return err
				}
				k = strings.TrimSpace(string(b))
			}
			if k == "" {
				return fmt.Errorf("key 为空；用法 a2v key sk-xxx")
			}
			p.Key = k
			if err := cfg.Save(); err != nil {
				return err
			}
			if err := codex.SyncKey(cfg, p); err != nil {
				fmt.Fprintln(os.Stderr, "a2v: 同步 codex 配置失败: "+err.Error())
			}
			fmt.Printf("%s 的 key 已更新为 %s\n", p.Name, config.MaskKey(k))
			return nil
		},
	}
}

// ---------------------------------------------------------------- sync

func newSyncCmd() *cobra.Command {
	var prune bool
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "从当前代理拉取模型列表",
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			p := cfg.Proxy()
			if p == nil {
				return fmt.Errorf("还没有代理，先 a2v proxy add")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			fmt.Printf("从 %s 拉取模型…\n", p.Name)
			list, err := remote.Sync(ctx, p)
			if err != nil {
				return err
			}
			p.SetModels(list)
			before := len(cfg.Models)
			cfg.Models = config.MergeHistory(cfg.Models, list)
			fresh := len(cfg.Models) - before
			if prune {
				cfg.Models = config.PruneTo(cfg.Models, list)
			}
			if cfg.Model == "" && len(list) > 0 {
				cfg.Model = list[0].ID
			}
			if err := cfg.Save(); err != nil {
				return err
			}
			hidden := 0
			for _, m := range cfg.Models {
				if !p.Authorised(m.ID) {
					hidden++
				}
			}
			fmt.Printf("%s 的 key 授权 %d 个模型（新增 %d 个，未授权 %d 个）\n", p.Name, len(list), fresh, hidden)
			if prune {
				fmt.Println("已按授权列表清理历史目录")
			} else if hidden > 0 {
				fmt.Println("未授权的旧模型已折叠；a2v sync --prune 可彻底清掉")
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&prune, "prune", false, "按当前 key 的授权列表清理历史目录")
	return cmd
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ---------------------------------------------------------------- sessions

func newSessionsCmd() *cobra.Command {
	var limit int
	var all bool
	cmd := &cobra.Command{
		Use:   "sessions",
		Short: "列出可恢复的 codex 会话（默认排除子代理）",
		RunE: func(_ *cobra.Command, _ []string) error {
			fetch := codex.Recent
			if all {
				fetch = codex.RecentAll
			}
			list, err := fetch(limit)
			if err != nil {
				return err
			}
			if len(list) == 0 {
				fmt.Println("(空)")
				return nil
			}
			for _, st := range list {
				model := st.Model
				if len(model) > 18 {
					model = model[:17] + "…"
				}
				tag := ""
				if st.Subagent {
					tag = "  子代理"
				}
				fmt.Printf("  %s  %-18s %s%s\n      %s\n", st.Time.Format("01-02 15:04"), model, st.ID, tag, st.Title)
			}
			if !all {
				fmt.Println("\n子代理会话已排除；a2v sessions --all 可查看")
			}
			fmt.Println("恢复：a2v resume <id>")
			return nil
		},
	}
	cmd.Flags().IntVarP(&limit, "limit", "n", 15, "显示条数")
	cmd.Flags().BoolVarP(&all, "all", "a", false, "包含子代理（subagent）会话")
	return cmd
}

// ---------------------------------------------------------------- resume / pin

func newResumeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "resume <session-id>",
		Short: "用当前模型恢复一个 codex 会话",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			model := cfg.Model
			cwd, _ := os.Getwd()
			if _, pin := config.LocalPath(cwd); pin != nil && pin.Model != "" {
				model = cfg.Resolve(pin.Model)
			}
			if model == "" && len(cfg.Models) > 0 {
				model = cfg.Models[0].ID
			}
			return launch(cfg, model, args[0], nil)
		},
	}
}

func newPinCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "pin <model|->",
		Short: "把当前目录固定到某个模型（- 表示清除）",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			cwd, _ := os.Getwd()
			if args[0] == "-" || args[0] == "off" {
				path, pin := config.LocalPath(cwd)
				if pin == nil {
					fmt.Println("当前目录没有固定")
					return nil
				}
				if path != filepath.Join(cwd, ".a2v.json") {
					return fmt.Errorf("%s 不是本目录的文件，请手动删除", path)
				}
				if err := os.Remove(path); err != nil {
					return err
				}
				fmt.Println("已清除目录固定")
				return nil
			}
			model := cfg.Resolve(args[0])
			path := filepath.Join(cwd, ".a2v.json")
			if err := config.WriteLocal(path, &config.Local{Model: model}); err != nil {
				return err
			}
			fmt.Printf("本目录固定为 %s（写入 %s）\n", model, path)
			return nil
		},
	}
}

// ---------------------------------------------------------------- doctor

func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "检查环境、配置和代理连通性",
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			ok := true
			line := func(good bool, label, detail string) {
				mark := "✓"
				if !good {
					mark = "✗"
					ok = false
				}
				fmt.Printf("  %s %-22s %s\n", mark, label, detail)
			}

			fmt.Println("a2v " + version.Version + "  (" + runtime.GOOS + "/" + runtime.GOARCH + ")")
			fmt.Println()
			line(true, "配置文件", config.Path())

			bin, err := codex.Bin()
			if err != nil {
				line(false, "codex", err.Error())
			} else {
				line(true, "codex", bin)
				if out, e := exec.Command(bin, "--version").Output(); e == nil {
					ver := strings.TrimSpace(string(out))
					caps := []string{}
					if codex.Supports("resume") {
						caps = append(caps, "resume")
					}
					if codex.Supports("--dangerously-bypass-approvals-and-sandbox") {
						caps = append(caps, "新权限参数")
					} else if codex.Supports("--dangerously-skip-permissions") {
						caps = append(caps, "旧权限参数")
					}
					line(true, "codex 版本", ver+"（"+strings.Join(caps, " / ")+"；a2v 按能力自动适配）")
				}
			}

			p := cfg.Proxy()
			if p == nil {
				line(false, "当前代理", "未设置")
			} else {
				line(p.Key != "", "当前代理", fmt.Sprintf("%s  %s  key=%s", p.Name, p.URL, config.MaskKey(p.Key)))
				ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
				code, err := remote.Health(ctx, p)
				cancel()
				if err != nil {
					line(false, "代理连通性", err.Error())
				} else {
					line(code == 200, "代理连通性", fmt.Sprintf("GET /v1/models -> HTTP %d", code))
				}
			}

			eff := codex.EffectiveFlags(cfg.Flags)
			bad := codex.UnsupportedFlags(cfg.Flags)
			switch {
			case len(bad) > 0:
				line(false, "codex 参数", "codex 不认: "+strings.Join(bad, " ")+"（改 config 的 flags）")
			case eff != "" && eff != cfg.Flags:
				line(true, "codex 参数", eff+"  ← 已从旧名自动转换")
			case eff != "":
				line(true, "codex 参数", eff)
			}

			line(len(cfg.Models) > 0, "模型目录", fmt.Sprintf("%d 个（含历史）", len(cfg.Models)))
			if p != nil && len(p.Models) > 0 {
				line(true, "key 授权", fmt.Sprintf("%d 个（同步于 %s）", len(p.Models), p.SyncedAt))
			} else if p != nil {
				line(false, "key 授权", "尚未同步，运行 a2v sync")
			}

			cwd, _ := os.Getwd()
			if path, pin := config.LocalPath(cwd); pin != nil {
				line(true, "目录固定", fmt.Sprintf("%s → %s", path, pin.Model))
			} else {
				line(true, "目录固定", "无（用 a2v pin <model> 创建）")
			}

			home, _ := os.UserHomeDir()
			if _, err := os.Stat(filepath.Join(home, ".codex", "auth.json")); err == nil {
				line(true, "codex auth.json", "存在")
			} else {
				line(true, "codex auth.json", "不存在（a2v 仍通过环境变量注入 key）")
			}

			list, _ := codex.Recent(1)
			line(true, "会话目录", fmt.Sprintf("%s（最近 %d 条可恢复）", codex.SessionsDir(), len(list)))

			fmt.Println()
			if ok {
				fmt.Println("一切正常。")
			} else {
				return fmt.Errorf("有检查项未通过")
			}
			return nil
		},
	}
}

// ---------------------------------------------------------------- update

type manifest struct {
	Version string            `json:"version"`
	SHA256  map[string]string `json:"sha256"`
	URL     string            `json:"url"`
}

func newUpdateCmd() *cobra.Command {
	var from string
	cmd := &cobra.Command{
		Use:   "update",
		Short: "从 tool.kervein.club/a2v 更新自身",
		RunE: func(_ *cobra.Command, _ []string) error {
			base := from
			if base == "" {
				base = version.Base
			}
			m, err := fetchManifest(base)
			if err != nil {
				return err
			}
			if m.Version == version.Version {
				fmt.Printf("已是最新版本 %s\n", version.Version)
				return nil
			}
			key := runtime.GOOS + "-" + runtime.GOARCH
			url := m.URL
			if url == "" {
				url = fmt.Sprintf("%s/a2v-%s", base, key)
			}
			fmt.Printf("发现 %s（当前 %s），下载 %s\n", m.Version, version.Version, url)
			if _, err := download(url, m.SHA256[key]); err != nil {
				return err
			}
			fmt.Println("已更新，重新运行 a2v 生效")
			return nil
		},
	}
	cmd.Flags().StringVar(&from, "from", "", "改用其它分发地址")
	return cmd
}

func fetchManifest(base string) (*manifest, error) {
	c := &http.Client{Timeout: 15 * time.Second}
	resp, err := c.Get(base + "/latest.json")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("拉取 latest.json 失败: HTTP %d", resp.StatusCode)
	}
	var m manifest
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		return nil, err
	}
	return &m, nil
}

// download fetches to a temp file beside the running binary, verifies the
// checksum, then swaps it in.
func download(url, want string) (string, error) {
	self, err := os.Executable()
	if err != nil {
		return "", err
	}
	self, _ = filepath.EvalSymlinks(self)

	c := &http.Client{Timeout: 5 * time.Minute}
	resp, err := c.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("下载失败: HTTP %d", resp.StatusCode)
	}

	tmp, err := os.CreateTemp(filepath.Dir(self), ".a2v-update-*")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmp, h), resp.Body); err != nil {
		tmp.Close()
		return "", err
	}
	tmp.Close()
	sum := hex.EncodeToString(h.Sum(nil))
	if want != "" && !strings.EqualFold(want, sum) {
		return "", fmt.Errorf("sha256 不符：本地 %s ≠ 清单 %s", sum, want)
	}
	if err := os.Chmod(tmpName, 0o755); err != nil {
		return "", err
	}
	if err := os.Rename(tmpName, self); err != nil {
		return "", fmt.Errorf("替换 %s 失败: %w", self, err)
	}
	return sum, nil
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "打印版本",
		Run: func(*cobra.Command, []string) {
			fmt.Printf("a2v %s (%s/%s)\n", version.Version, runtime.GOOS, runtime.GOARCH)
		},
	}
}

func isTTY() bool {
	st, err := os.Stdin.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}
