#!/usr/bin/env bash
# a2v installer — https://tool.kervein.club/a2v
#
#   curl -fsSL https://tool.kervein.club/a2v/install.sh | bash
#   curl -fsSL https://tool.kervein.club/a2v/install.sh | bash -s -- --dir ~/bin
#
# 只做三件事：认平台、下二进制、按 sha256 校验后放进 PATH。
set -euo pipefail

BASE="${A2V_BASE:-https://tool.kervein.club/a2v}"
VERSION="${A2V_VERSION:-}"
DEST=""

B=$'\033[1m'; D=$'\033[2m'; G=$'\033[32m'; Y=$'\033[33m'; R=$'\033[31m'; N=$'\033[0m'
[ -t 1 ] || { B=""; D=""; G=""; Y=""; R=""; N=""; }

say()  { printf '%s\n' "$*"; }
ok()   { printf '%s✓%s %s\n' "$G" "$N" "$*"; }
warn() { printf '%s!%s %s\n' "$Y" "$N" "$*" >&2; }
die()  { printf '%s✗%s %s\n' "$R" "$N" "$*" >&2; exit 1; }

while [ $# -gt 0 ]; do
    case "$1" in
        --dir)     DEST="${2:?--dir 需要路径}"; shift 2 ;;
        --version) VERSION="${2:?--version 需要版本}"; shift 2 ;;
        --base)    BASE="${2:?--base 需要 URL}"; shift 2 ;;
        -h|--help) sed -n '2,9p' "$0" 2>/dev/null || true; exit 0 ;;
        *) die "未知参数: $1" ;;
    esac
done

say ""
say "  ${B}▲ a2v${N} ${D}a2vec · codex launcher${N}"
say ""

# ---------------------------------------------------------------- 平台
os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch="$(uname -m)"
case "$arch" in
    x86_64|amd64)  arch="amd64" ;;
    aarch64|arm64) arch="arm64" ;;
    *) die "不支持的架构: $arch（目前提供 amd64 / arm64）" ;;
esac
case "$os" in
    linux|darwin) ;;
    *) die "不支持的系统: $os（目前提供 linux / darwin）" ;;
esac
ok "平台 $os-$arch"

# ---------------------------------------------------------------- 下载
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

url="$BASE/a2v-$os-$arch"
[ -n "$VERSION" ] && url="$BASE/releases/$VERSION/a2v-$os-$arch"
say "${D}↓ $url${N}"
curl -fsSL --retry 3 --connect-timeout 15 -o "$tmp/a2v" "$url" \
    || die "下载失败（检查网络或 --base）"

# ---------------------------------------------------------------- 校验
expect=""
if curl -fsSL --max-time 15 -o "$tmp/a2v.sha256" "$url.sha256" 2>/dev/null; then
    expect="$(awk '{print $1; exit}' "$tmp/a2v.sha256")"
fi
[ -n "$expect" ] || die "拿不到 sha256（$url.sha256），拒绝安装未校验的二进制"

if command -v sha256sum >/dev/null 2>&1; then
    got="$(sha256sum "$tmp/a2v" | awk '{print $1}')"
elif command -v shasum >/dev/null 2>&1; then
    got="$(shasum -a 256 "$tmp/a2v" | awk '{print $1}')"
else
    die "系统里没有 sha256sum / shasum，无法校验"
fi
[ "$got" = "$expect" ] || die "sha256 不符（本地 $got ≠ 期望 $expect），可能下载被篡改"
ok "sha256 ${got:0:12}…"

# ---------------------------------------------------------------- 安装
if [ -z "$DEST" ]; then
    for d in /usr/local/bin "$HOME/.local/bin"; do
        if [ -d "$d" ] && [ -w "$d" ]; then DEST="$d"; break; fi
    done
fi
if [ -z "$DEST" ]; then
    if [ "$(id -u)" = 0 ]; then
        DEST=/usr/local/bin
    elif sudo -n true 2>/dev/null; then
        DEST=/usr/local/bin
    else
        DEST="$HOME/.local/bin"
    fi
fi
mkdir -p "$DEST" 2>/dev/null || true
[ -w "$DEST" ] || die "$DEST 不可写；用 --dir 指定一个可写目录，或以 root 运行"

install -m 755 "$tmp/a2v" "$DEST/a2v.tmp.$$" 2>/dev/null || cp "$tmp/a2v" "$DEST/a2v.tmp.$$"
mv -f "$DEST/a2v.tmp.$$" "$DEST/a2v"
ok "已安装 $DEST/a2v ($("$DEST/a2v" version 2>/dev/null | head -1))"

# ---------------------------------------------------------------- 收尾
case ":$PATH:" in
    *":$DEST:"*) ;;
    *) warn "$DEST 不在 PATH 里，加一行到 ~/.zshrc 或 ~/.bashrc："
       warn "  export PATH=\"$DEST:\$PATH\"" ;;
esac

say ""
if [ -f "$HOME/.veicrun/veicrun.json" ] && [ ! -f "$HOME/.config/a2v/config.json" ]; then
    say "  ${D}检测到 veicrun 配置，首次运行 a2v 会自动迁移。${N}"
fi
say "  ${B}下一步${N}"
say "    a2v            ${D}# 交互菜单：选模型 / 切代理 / 恢复会话${N}"
say "    a2v doctor     ${D}# 检查 codex、代理和配置文件${N}"
say "    a2v --help     ${D}# 全部命令${N}"
say ""
say "  ${D}文档 https://tool.kervein.club/a2v/${N}"
say ""
