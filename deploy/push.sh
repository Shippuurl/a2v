#!/usr/bin/env bash
# 把 a2v 的发布产物推到 ECS 静态站：tool.kervein.club/a2v/
# 用法: bash deploy/push.sh
set -euo pipefail

SRC="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ECS="${ECS_PY:-/media/shippuu/A2Vec/toolchains/ecs.py}"
REMOTE_DIR="/var/www/a2v"
BASE="https://tool.kervein.club/a2v"

[ -f "$ECS" ] || { echo "找不到 ecs.py: $ECS" >&2; exit 1; }
[ -f "$SRC/dist/latest.json" ] || { echo "先跑 ./build.sh" >&2; exit 1; }

echo "==> 上传到 $REMOTE_DIR"
python3 "$ECS" run "mkdir -p $REMOTE_DIR" >/dev/null

shopt -s nullglob
for f in "$SRC"/dist/a2v-* "$SRC"/dist/latest.json; do
    python3 "$ECS" put "$f" "$REMOTE_DIR/$(basename "$f")" >/dev/null
    echo "  ↑ $(basename "$f")"
done
python3 "$ECS" put "$SRC/install.sh" "$REMOTE_DIR/install.sh" >/dev/null
echo "  ↑ install.sh"
python3 "$ECS" put "$SRC/install.ps1" "$REMOTE_DIR/install.ps1" >/dev/null
echo "  ↑ install.ps1"
python3 "$ECS" put "$SRC/index.html" "$REMOTE_DIR/index.html" >/dev/null
echo "  ↑ index.html"
[ -f "$SRC/README.md" ] && python3 "$ECS" put "$SRC/README.md" "$REMOTE_DIR/README.md" >/dev/null && echo "  ↑ README.md"

python3 "$ECS" run "chmod 755 $REMOTE_DIR/a2v-*; chmod 644 $REMOTE_DIR/*.sha256 $REMOTE_DIR/latest.json $REMOTE_DIR/install.sh $REMOTE_DIR/install.ps1 $REMOTE_DIR/index.html $REMOTE_DIR/README.md; ls -lh $REMOTE_DIR | tail -n +2" | tail -16

echo "==> 验收"
sleep 2
local_sha=$(sha256sum "$SRC/install.sh" | awk '{print $1}')
remote_sha=$(curl -fsSL --max-time 30 "$BASE/install.sh" | sha256sum | awk '{print $1}')
[ "$local_sha" = "$remote_sha" ] && echo "  ✓ install.sh sha256 一致" || { echo "  ✗ install.sh 不一致" >&2; exit 1; }

bin_local=$(sha256sum "$SRC/dist/a2v-linux-amd64" | awk '{print $1}')
bin_remote=$(curl -fsSL --max-time 60 "$BASE/a2v-linux-amd64" | sha256sum | awk '{print $1}')
[ "$bin_local" = "$bin_remote" ] && echo "  ✓ a2v-linux-amd64 sha256 一致" || { echo "  ✗ 二进制不一致" >&2; exit 1; }

for u in "$BASE/" "$BASE/install.sh" "$BASE/latest.json" "$BASE/a2v-linux-amd64.sha256"; do
    printf '  %-42s ' "${u#https://tool.kervein.club}"
    curl -s -o /dev/null -w '%{http_code} %{content_type} %{size_download}B\n' --max-time 30 "$u"
done

echo
echo "上线完成："
echo "  说明页  $BASE/"
echo "  安装    curl -fsSL $BASE/install.sh | bash"
echo "  自更新  a2v update"
