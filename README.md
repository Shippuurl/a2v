# a2v

> a2vec 的命令行启动器。把 `codex`、多个中转代理、模型目录、会话恢复收进一个终端菜单。

[![MIT](https://img.shields.io/badge/license-MIT-5eead4.svg)](LICENSE)
[![Go](https://img.shields.io/badge/go-1.24%2B-818cf8.svg)](go.mod)
[![install](https://img.shields.io/badge/install-curl%20%7C%20bash-c084fc.svg)](https://tool.kervein.club/a2v/)

```
  ▲  a2v  a2vec · codex launcher
  ──────────────────────────────────────────────────────
  proxy my-relay  model gpt-5.6-sol   dir ~/project

  ▌  31  GPT 5.6 Sol  GPT 代码                              g
     32  GPT 5.6 Sol
     33  GPT 5.6 Terra  GPT 重量                          terra
     34  GPT 6
     35  GPT 6 Astra
  ──────────────────────────────────────────────────────
  ↑↓ 选择 · ⏎ 启动 · / 筛选 · p 代理 · r 会话 · ? 帮助 · q 退出
```

## 安装

```bash
curl -fsSL https://tool.kervein.club/a2v/install.sh | bash
```

源码：[github.com/Shippuurl/a2v](https://github.com/Shippuurl/a2v)

Linux / macOS，amd64 / arm64。脚本只做三件事：认平台、下二进制、按 `sha256` 校验后放进 `PATH`（优先 `/usr/local/bin`，其次 `~/.local/bin`）。

首次运行自带一个中转端点 **https://new.8w8.shop**（只有地址，不带任何 key），
粘贴你自己的 API Key 后会自动拉取该 key 授权的模型列表。不想要它：
`a2v proxy rm new8w8`，再 `a2v proxy add` 换成自己的中转。

自更新：`a2v update`。指定目录：`... | bash -s -- --dir ~/bin`。

## 用法

```bash
a2v                     # 交互菜单
a2v sol                 # 用别名/模型 id 直接启动
a2v run gpt-6-astra -- --full-auto   # 参数透传给 codex
a2v proxy list          # 代理：list / use / add / rm <id>
a2v key sk-xxx          # 写 key，并同步 ~/.codex/auth.json
a2v sync                # 按当前 key 拉取授权模型（--prune 清理历史）
a2v models rm <id>      # 删除目录里的模型（历史/授权都删）
a2v models prune        # 一次删掉当前 key 未授权的历史模型
a2v pin gpt-6-astra     # 当前目录固定模型（写 ./.a2v.json，- 清除）
a2v sessions            # 最近会话（默认排除子代理；--all 含子代理）
a2v resume <session-id> # 用当前模型恢复会话
a2v doctor              # 体检
a2v update              # 自更新
```

非交互环境（没有 TTY）自动退化成编号菜单，`echo 3 | a2v` 也能用。

## 模型列表 = 当前 key 的授权列表

`a2v sync` 拉到什么就显示什么：同步结果**按代理分别保存**（`proxies[].models` + `synced_at`），
切代理时菜单跟着变，不会把 A 代理的模型混到 B 代理下面。

历史模型不会丢，但默认折叠：

- 默认只显示当前 key 授权的模型
- 未授权的旧条目收进「历史」，按 `t` 展开，行尾标 `未授权`
- `a2v sync --prune` 彻底删掉历史，只留授权列表
- 从未同步过的代理（`models` 为空）不做过滤，按全量目录显示，并提示去 `sync`

## 界面

| 键 | 作用 |
|---|---|
| `↑ ↓ / j k` | 移动 |
| `⏎` | 启动 codex |
| `/` | 模糊筛选 |
| `p` | 代理列表 / 新增 |
| `ctrl+k` | 设置 API Key |
| `r` | 恢复会话 |
| `a` | 会话页：显示 / 隐藏子代理会话 |
| `d` | 删除选中的模型或代理（`y` 确认） |
| `s` | 重新拉取模型（按 key 授权） |
| `t` | 展开 / 折叠未授权的历史模型 |
| `?` | 帮助 |
| `q` | 退出 |

### 子代理会话

codex 会给子代理线程写 `thread_source: "subagent"`，并在 `source` 里记 `parent_thread_id`。
`a2v` 默认把这类会话从恢复列表里滤掉（本机 124 个 rollout 里有 16 个是子代理），
`a` 键或 `a2v sessions --all` 可以把它们放回来，行尾标「子代理」。

## 文件

| 路径 | 说明 |
|---|---|
| `~/.config/a2v/config.json` | 全局配置（0600，含 key） |
| `./.a2v.json` | 目录级固定 `{"model":"..."}` |
| `~/.veicrun/veicrun.json` | 旧版配置，首次运行自动迁移（只读，不改原文件） |
| `~/.codex/sessions` | 会话来源，只读 |

配置结构：

```json
{
  "version": 1,
  "model": "gpt-5.6-sol",
  "flags": "--dangerously-skip-permissions",
  "active_proxy": "my-relay",
  "proxies": [{"id":"my-relay","name":"My Relay","url":"https://relay.example.com","key":"sk-..."}],
  "models": [{"id":"gpt-5.6-sol","name":"GPT 5.6 Sol","desc":"GPT 代码"}],
  "aliases": {"sol":"gpt-5.6-sol","terra":"gpt-5.6-terra"},
  "recent": ["gpt-6-astra"]
}
```

## 启动 codex 时到底做了什么

```bash
codex -c model_provider="proxy" \
      -c model_providers.proxy.name="<代理名>" \
      -c model_providers.proxy.base_url="<url>/v1" \
      -c model_providers.proxy.env_key="CODEX_PROXY_API_KEY" \
      -c model_providers.proxy.wire_api="responses" \
      <config.flags> --model <model>
```

环境变量：`OPENAI_API_KEY` / `CODEX_PROXY_API_KEY` / `FASTLLM_API_KEY` / `OPENAI_BASE_URL`。
没有代理时直接 `codex --model ...`，走 `~/.codex/config.toml` 的默认 provider。

### 兼容不同版本的 codex

a2v 每次启动前读一遍 `codex --help`，按**当前装的那个 codex** 能认什么来选参数名：

| 配置里写的 | 新 codex（0.16x+） | 老 codex |
|---|---|---|
| `--dangerously-skip-permissions` | 自动换成 `--dangerously-bypass-approvals-and-sandbox` | 原样传 |
| `--dangerously-bypass-approvals-and-sandbox` | 原样传 | 自动退回 `--dangerously-skip-permissions` |

同理处理 `--auto-approve` / `--full-auto`。如果老版本没有 `resume` 子命令，
恢复会话会退化成开新会话，而不是报错。探测不到 codex 时（比如没装）
参数原样传，不做猜测。`a2v doctor` 会显示 codex 版本、识别到的能力，
以及最终生效的参数；完全不认的参数也会在那里报出来。

## 开发与发布

```bash
./build.sh                 # 交叉编译 + sha256 + latest.json
VERSION=v1.1.0 ./build.sh  # 指定版本
```

部署（本仓库的 `deploy/`）：

```bash
python3 deploy/patch-nginx-a2v.py   # 首次：给 tool.kervein.club 加 /a2v/ location
bash deploy/push.sh                 # 上传 dist/ + install.sh + index.html 并验收
```

技术栈：Go + [Bubble Tea](https://github.com/charmbracelet/bubbletea) / [Lip Gloss](https://github.com/charmbracelet/lipgloss) / [Cobra](https://github.com/spf13/cobra)。

## License

[MIT](LICENSE) © 2026 Shippuurl
