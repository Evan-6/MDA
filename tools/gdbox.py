"""準備 GDBOX SDK（git submodule）與它的 gdb.dll。

`agent/go-service/gdbox-sdk` 是 FirmwareManageTool_Rust 的 git submodule，釘在特定 commit；
Go 綁定沒有副本，`agent/go-service/go.mod` 的 replace 直接指向它的 sdk/go，所以 SDK 永遠
是上游那一份，不會漂移。

    python tools/gdbox.py           # 檢查 submodule、編 gdb.dll、回報釘住的 commit
    python tools/gdbox.py --check   # 只檢查（就位、dll 存在、C ABI 一致），不改檔案
    python tools/gdbox.py --test    # 另外跑 SDK 自己的 Go 測試

編好的 dll 落在 `agent/go-service/gdbox-sdk/target/release/gdb.dll`（不進版控）；
`tools/install.py` 會把它複製到 install/agent/，缺少時硬體輸入退回內建控制。

submodule 沒初始化時 `go build` 會因為 replace 指到空目錄而失敗，先跑這個腳本。
"""

from __future__ import annotations

import argparse
import os
import re
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
SUBMODULE = ROOT / "agent" / "go-service" / "gdbox-sdk"
BINDING_DIR = SUBMODULE / "sdk" / "go"
HEADER = SUBMODULE / "sdk" / "ffi" / "include" / "gdb.h"
DLL = SUBMODULE / "target" / "release" / "gdb.dll"


def run(command: list[str], cwd: Path | None = None, env: dict | None = None) -> subprocess.CompletedProcess:
    return subprocess.run(command, cwd=cwd, capture_output=True, text=True, env=env)


def submodule_state() -> tuple[bool, str]:
    """回傳 (就位與否, 說明)。未初始化或指向別的 commit 都要講清楚。"""
    if not (BINDING_DIR / "gdb.go").exists():
        return False, f"{SUBMODULE} 沒有內容，先跑 git submodule update --init --recursive"
    status = run(["git", "submodule", "status", str(SUBMODULE.relative_to(ROOT))], cwd=ROOT)
    line = status.stdout.rstrip("\n")
    if not line.strip():
        return True, "submodule 狀態不明（不在版控裡？）"
    marker = line[0]
    commit = line.split()[0].lstrip("+-")
    notes = {
        " ": f"釘在 {commit[:8]}",
        "+": f"工作區在 {commit[:8]}，與父 repo 記錄的不同（submodule 被改過）",
        "-": f"未初始化（記錄 {commit[:8]}）",
        "U": "有未解決的 submodule 衝突",
    }
    return marker in " +", notes.get(marker, f"狀態 {marker}")


def abi_versions() -> tuple[str | None, str | None]:
    header = HEADER.read_text(encoding="utf-8") if HEADER.exists() else ""
    binding = (BINDING_DIR / "gdb.go").read_text(encoding="utf-8") if (BINDING_DIR / "gdb.go").exists() else ""
    header_match = re.search(r"#define\s+GDB_ABI_VERSION\s+(\d+)", header)
    binding_match = re.search(r"abiVersion\s*=\s*(\d+)", binding)
    return (header_match.group(1) if header_match else None, binding_match.group(1) if binding_match else None)


def build_library() -> bool:
    cargo = "cargo"
    result = run([cargo, "build", "-p", "gdb-ffi", "--release"], cwd=SUBMODULE)
    if result.returncode != 0:
        print((result.stderr or result.stdout).strip()[-800:])
        print("cargo build 失敗；沒有 Rust 工具鏈就無法重建 gdb.dll")
        return False
    return True


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--check", action="store_true", help="只檢查，不編譯")
    parser.add_argument("--test", action="store_true", help="另外跑 SDK 的 Go 測試")
    args = parser.parse_args()

    ready, note = submodule_state()
    print(f"submodule: {note}")
    if not ready:
        return 1

    header_abi, binding_abi = abi_versions()
    if header_abi and binding_abi and header_abi != binding_abi:
        print(f"ABI 不一致：gdb.h={header_abi} gdb.go={binding_abi}")
        return 1
    print(f"C ABI: {binding_abi or header_abi}")

    if not args.check:
        if not build_library():
            return 1
        print(f"library: {DLL.relative_to(ROOT)} ({DLL.stat().st_size} bytes)")
    elif not DLL.exists():
        print(f"沒有 {DLL.relative_to(ROOT)}；跑 python tools/gdbox.py 編一份")
        return 1

    if args.test:
        # SDK 的測試從 sdk/go 執行，dll 在 target/release，靠 GDBOX_LIBRARY 指定。
        env = {**os.environ, "GDBOX_LIBRARY": str(DLL.resolve())}
        result = run(["go", "test", "./..."], cwd=BINDING_DIR, env=env)
        if result.returncode != 0:
            print((result.stdout + result.stderr).strip()[-1500:])
            return 1
        print("SDK Go 測試通過")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
