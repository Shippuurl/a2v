# a2v

> a2vec 的命令行启动器。把 `codex`、多个中转代理、模型目录、会话恢复收进一个终端菜单。

```
  ▲  a2v  a2vec · codex launcher
  ──────────────────────────────────────────────────────
  proxy new8w8   model gpt-5.6-sol   dir ~/project

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

Linux / macOS，amd64 / arm64。脚本只做三件事：认平台、下二进制、按 `sha256` 校验后放进 `PATH`（优先 `/usr/local/bin`，其次 `~/.local/bin`）。

自更新：`a2v update`。指定目录：`... | bash -s -- --dir ~/bin`。

## 用法

```bash
a2v                     # 交互菜单
a2v sol                 # 用别名/模型 id 直接启动
a2v run gpt-6-astra -- --full-auto   # 参数透传给 codex
a2v proxy list          # 代理：list / use / add / rm
a2v key sk-xxx          # 写 key，并同步 ~/.codex/auth.json
a2v sync                # 从当前代理拉模型列表
a2v pin gpt-6-astra     # 当前目录固定模型（写 ./.a2v.json，- 清除）
a2v sessions            # 最近会话
a2v resume <session-id> # 用当前模型恢复会话
a2v doctor              # 体检
a2v update              # 自更新
```

非交互环境（没有 TTY）自动退化成编号菜单，`echo 3 | a2v` 也能用。

## 界面

| 键 | 作用 |
|---|---|
| `↑ ↓ / j k` | 移动 |
| `⏎` | 启动 codex |
| `/` | 模糊筛选 |
| `p` | 代理列表 / 新增 |
| `ctrl+k` | 设置 API Key |
| `r` | 恢复会话 |
| `s` | 重新拉取模型 |
| `?` | 帮助 |
| `q` | 退出 |

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
  "active_proxy": "new8w8",
  "proxies": [{"id":"new8w8","name":"new8w8","url":"https://new.8w8.shop","key":"sk-..."}],
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

MIT
