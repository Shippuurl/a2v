#!/usr/bin/env python3
"""给 tool.kervein.club 的 443 server 块插入 /a2v/ 静态目录。

与 /jetson/ 同样的注意点：
  - 必须插在 443 块里（80 块是 301）
  - types 用 include mime.types，避免 svg/woff 变 default_type
  - 二进制按 octet-stream，install.sh / latest.json 按文本
  - 幂等，nginx -t 失败自动回滚
"""
import shutil
import subprocess
import sys
import time

CONF = "/etc/nginx/sites-enabled/tool.kervein.club.conf"
MARK = "# ---- a2v 工具（二进制 + install.sh + 说明页）----"
BLOCK = f"""
    {MARK}
    location ^~ /a2v/ {{
        alias /var/www/a2v/;
        index index.html;
        autoindex on;
        charset utf-8;
        include /etc/nginx/mime.types;
        default_type application/octet-stream;
        add_header Content-Disposition "inline" always;
        add_header Cache-Control "no-cache" always;
        include /etc/nginx/snippets/a2vec-security.conf;
        location ~ \\.sh$ {{ default_type text/plain; charset utf-8; }}
        location ~ \.ps1$ {{ default_type text/plain; charset utf-8; }}
        location ~ \\.json$ {{ default_type application/json; charset utf-8; }}
        location ~ \\.html$ {{ default_type text/html; charset utf-8; }}
        location ~ \\.md$ {{ default_type text/plain; charset utf-8; }}
    }}
"""

src = open(CONF, encoding="utf-8").read()

if MARK in src:
    lines = src.splitlines(keepends=True)
    kept, skip, depth = [], False, 0
    for l in lines:
        if MARK in l:
            skip, depth = True, 0
            continue
        if skip:
            depth += l.count("{") - l.count("}")
            if depth <= 0 and "}" in l:
                skip = False
            continue
        kept.append(l)
    src = "".join(kept)
    print("已移除旧的 /a2v/ 块")

lines = src.splitlines(keepends=True)
idx = None
for i, l in enumerate(lines):
    if l.strip().startswith("root /var/www/tool.kervein.club/current;"):
        idx = i + 1
        break
if idx is None:
    print("FAIL: 没找到 443 server 块的 root 行")
    sys.exit(1)

subprocess.run(["mkdir", "-p", "/root/nginx-backups"], check=False)
bak = "/root/nginx-backups/tool.kervein.club.conf.bak-" + time.strftime("%Y%m%d-%H%M%S")
shutil.copy2(CONF, bak)
open(CONF, "w", encoding="utf-8").write("".join(lines[:idx]) + BLOCK + "".join(lines[idx:]))

t = subprocess.run(["nginx", "-t"], capture_output=True, text=True)
print(t.stderr.strip()[-300:])
if t.returncode != 0:
    shutil.copy2(bak, CONF)
    print("FAIL: nginx -t 失败，已回滚")
    sys.exit(1)
subprocess.run(["systemctl", "reload", "nginx"], check=True)
print("OK: 已写入 443 块并 reload；备份", bak)
