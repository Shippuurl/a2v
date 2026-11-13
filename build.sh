#!/usr/bin/env bash
# 构建发布产物：dist/a2v-<os>-<arch>、.sha256、latest.json
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")"
VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo v1.0.0)}"
export PATH="$HOME/.local/go/bin:$PATH"
export CGO_ENABLED=0
export GOPROXY="${GOPROXY:-https://goproxy.cn,direct}"

mkdir -p dist
rm -f dist/a2v-* dist/latest.json

TARGETS="${TARGETS:-linux/amd64 linux/arm64 darwin/amd64 darwin/arm64}"

echo "==> a2v $VERSION"
for t in $TARGETS; do
    os="${t%/*}"; arch="${t#*/}"
    out="dist/a2v-$os-$arch"
    echo "  build $out"
    GOOS="$os" GOARCH="$arch" go build -trimpath -buildvcs=false \
        -ldflags "-s -w -X github.com/Shippuurl/a2v/internal/version.Version=$VERSION" \
        -o "$out" ./cmd/a2v
    if command -v sha256sum >/dev/null 2>&1; then
        (cd dist && sha256sum "a2v-$os-$arch" > "a2v-$os-$arch.sha256")
    else
        (cd dist && shasum -a 256 "a2v-$os-$arch" > "a2v-$os-$arch.sha256")
    fi
done

echo "==> latest.json"
python3 - "$VERSION" <<'PY'
import hashlib, json, os, sys
ver = sys.argv[1]
files, sha = {}, {}
for name in sorted(os.listdir("dist")):
    if not name.startswith("a2v-"):
        continue
    if name.endswith(".sha256"):
        continue
    key = name[len("a2v-"):]
    p = os.path.join("dist", name)
    h = hashlib.sha256(open(p, "rb").read()).hexdigest()
    sha[key] = h
    files[key] = {"sha256": h, "size": os.path.getsize(p)}
doc = {"version": ver, "sha256": sha, "files": files}
json.dump(doc, open("dist/latest.json", "w"), indent=2, ensure_ascii=False)
print(json.dumps({"version": ver, "targets": list(sha)}, ensure_ascii=False))
PY

printf '\n产物：\n'
ls -lh dist/ | tail -n +2
