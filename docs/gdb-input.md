# 硬體滑鼠輸入（RP2040 / XIAO，GDBOX SDK）

MDA 預設用 MaaFramework 的 Win32 控制方式送出滑鼠與鍵盤事件；啟用本模式後，**輸入改由 RP2040
板子（XIAO RP2040）以實體 HID 送出**，截圖仍走原本的 Win32 擷取。適合不想安裝 Interception
驅動、或想讓輸入看起來像真實滑鼠的環境。

> 這是 opt-in 功能。偵測不到裝置、`gdb.dll` 不存在、或任何一步失敗時，任務仍會照原本的內建控制
> 方式執行，只會在日誌留下警告。

## 架構：MDA 不自己實作協議

通訊一律走 [FirmwareManageTool_Rust](https://github.com/Evan-6/FirmwareManageTool_Rust) 的
**GDBOX SDK**，MDA 只保留「pipeline 動作改寫」與「視窗座標」兩層：

| 層         | 位置                                                                      | 職責                                                                          |
| ---------- | ------------------------------------------------------------------------- | ----------------------------------------------------------------------------- |
| SDK C ABI  | `agent/go-service/gdbox-sdk`（git submodule）的 `target/release/gdb.dll`  | 列舉裝置、session／租約／心跳／重連、相對＋絕對移動、按鍵、STATUS、統一錯誤碼 |
| Go 綁定    | `agent/go-service/gdbox-sdk/sdk/go`（submodule 內，`replace` 直接指過去） | `syscall.NewLazyDLL("gdb.dll")`，無 cgo                                       |
| MDA 包裝層 | `agent/go-service/gdbinput`                                               | pipeline 動作改寫、自訂動作、影像→螢幕座標、遊戲視窗前景綁定                  |

- SDK 的參考實作是 Rust（`sdk/rust`）＋ C ABI（`sdk/ffi`）＋ C++／Python／Go 綁定，四者共用同一份
  行為。MDA 用 Go 綁定，行為與 SDK 的 Rust 端一致。
- 目前釘的 SDK 是 **C ABI 1／minor 10**：MDA 用到 `gdb_mouse_click_timed`（可指定 hold）、
  `gdb_mouse_move_to_on_report`（回報實際落點與 clamped）、`gdb_window_metrics`（client 區與 DPI）、
  `gdb_platform_prepare`、`gdb_device_last_error`，以及 client 區 anchor
  （`gdb_screen.kind = 4`，Go 的 `gdb.ScreenWindowClient(hwnd)`）。`gdb_open` 之後會把 ABI 版本與
  `gdb_capabilities()` 寫進日誌，方便對 issue。
- SDK 以 git submodule 釘版本（`agent/go-service/gdbox-sdk`），換版本就是搬 submodule 指標；
  `python tools/gdbox.py` 會檢查就位、編 `gdb.dll`、回報 C ABI 版本（`--check` 只檢查、`--test` 加跑 SDK 測試）。
  複製子模組要 `git clone --recursive`，已 clone 的跑 `git submodule update --init --recursive`。
- `gdb.dll` 必須與 `go-service.exe` 同目錄（LoadLibrary 只看 exe 目錄、system32、PATH）；
  `tools/install.py` 會自動複製。要放別處就設 `GDBOX_LIBRARY` 為完整路徑。

韌體（`FirmwareManageTool_Rust/firmware`）提供 Vendor HID 控制通道，report 64 bytes；
USB 身分 `0x1209:0x0B0B`（pid.codes，product `GDBOX KB8`）、usage `0xFF60:0x61`；實機版本 **3.2.0**，租約 2000 ms、poll 1 ms。
SDK 只枚舉這組 VID/PID，韌體換身分時 SDK 也要同步（`sdk/rust/src/lib.rs` 的 `VENDOR_ID`／`PRODUCT_ID`）。
韌體只有相對滑鼠，絕對定位由 SDK 在主機端閉環：`GetPhysicalCursorPos` → 算增量 →
`MOUSE_MOVE` → 收斂。

## 啟用

三種方式，優先序由高到低：

1. 環境變數 `MDA_GDB_INPUT=1`（或 `0` 強制關閉）
2. 設定檔 `%APPDATA%\MDA\gdb-input.json`
3. 預設值 `auto`：偵測到 SDK 認得的裝置就啟用

```json
{
    "enabled": "auto",
    "image_width": 1280,
    "image_height": 720,
    "window_class_regex": "UnityWndClass",
    "window_title_regex": "(NIKKE|勝利女神：妮姬)",
    "click_hold_ms": 40
}
```

`enabled` 可為 `auto` / `on` / `off`；`image_width` / `image_height` 必須與 pipeline 使用的辨識
影像尺寸一致（MDA 固定 1280×720）。視窗正則預設與 `interface.json` 的 Win32 控制器相同。

## 運作方式

1. **pipeline 完全不用改。** `gdbinput` 註冊一個 context sink，在每個任務第一個
   `Node.PipelineNode.Starting` 時拿到該任務的 `*Context`，呼叫 `HandleEntry`；用 task id 去重，
   每個任務只套用一次。任務入口節點原本的 `RuntimeQuotaCheck` 動作與這件事無關。
2. `HandleEntry`：SDK 列舉裝置 → 開 session（心跳與租約由 SDK 顧）→ 綁定並帶前景遊戲視窗 →
   把 `resource/pipeline/**/*.json` 的內建輸入動作改寫成 GDBOX 動作，再用 `ctx.OverridePipeline`
   套用到本次任務：

    | pipeline 動作       | 改寫成                    | 參數                                             | 底層 SDK 呼叫                                                        |
    | ------------------- | ------------------------- | ------------------------------------------------ | -------------------------------------------------------------------- |
    | `Click`             | `GdbClick`                | `target`、`target_offset`（原封不動）            | `gdb_mouse_move_to_on_report`（client 區）＋ `gdb_mouse_click_timed` |
    | `Swipe`             | `GdbSwipe`                | `begin`、`end`、`duration`、`end_hold`           | 兩次 `gdb_mouse_move_to_on_report` ＋按住拖曳                        |
    | `Scroll`            | `GdbScroll`               | `target`、`target_offset`、`dx`、`dy`            | `gdb_mouse_move_to_on_report` ＋ `gdb_mouse_wheel`                   |
    | `LongPress`         | `GdbLongPress`            | `target`、`target_offset`、`duration`、`contact` | `gdb_mouse_move_to_on_report` ＋ `gdb_mouse_click_timed`             |
    | `LongPressKey`      | `GdbLongPressKey`         | `key`、`duration`                                | `gdb_key_long_press`                                                 |
    | `ClickKey`          | `GdbClickKey`             | `key`（Windows 虛擬鍵碼）                        | `gdb_key_long_press`                                                 |
    | `KeyDown` / `KeyUp` | `GdbKeyDown` / `GdbKeyUp` | `key`                                            | `gdb_key_down` / `gdb_key_up`                                        |

    只有 `action` 區塊被替換，辨識、`next`、`pre_delay` / `post_delay` 由 MaaFramework 合併保留。
    虛擬鍵碼在 `gdbinput/override.go` 換成 HID page 7 的 usage，再交給 SDK。

    `Touch`／`TouchUp`／`MultiSwipe`／`InputText` 沒有覆寫：前兩者要按住不放的觸點語義，
    多點滑動與文字輸入在單一實體游標上做不到，這些節點在硬體模式仍走控制器的內建方式。

3. 點擊流程：`MoveToOnReport`（60 ms 線性、SDK 閉環收斂，回報實際落點）→ `ClickTimed`
   （按住 `click_hold_ms`，gap 1 ms）→ `BARRIER`（只保證 USB 已送出，不代表遊戲已處理）。
   落點被 anchor 夾住或誤差超過 2 px 時會記一筆 `pointer landed off target` 警告，
   內容含 requested／applied／error_px，不必再從「識別不到」反推是點擊還是辨識的問題。
4. 任務結束（Succeeded／Failed）時 `CloseSession()` 先 `ReleaseAll` 再關 session，避免留下按住的鍵。

## 多螢幕

座標是虛擬桌面的實體像素，SDK 的絕對移動只把落點「夾進」指定的矩形。`gdbinput` 傳的是遊戲視窗的
**client 區**（`gdb.ScreenWindowClient(hwnd)`，kind 4），所以視窗在主螢幕、副螢幕或負座標上都落在
同一個 client 座標系；把 anchor 換成預設的主螢幕，副螢幕的目標就會被夾回主螢幕邊緣
（實測 x=-2200 落在 x=-1）。用 client 區而不是外框，是因為外框含標題列與 Win10/11 的隱形 resize
邊框，貼邊的目標會被夾到看不見的邊框上。

client 原點、client 大小與 DPI 由 SDK 的 `gdb_window_metrics` 提供（`gdbinput` 不再自己做
`ClientToScreen`），換算影像座標與 SDK 的落點因此共用同一套 per-monitor v2 規則。

`gdbinput` 每 5 秒重抓一次視窗（`windowRefresh`）。任務中把視窗拖到別的螢幕，最晚 5 秒後才會用
新的原點換算，這段時間的點擊會落在舊位置。

## 限制與注意事項

- **只能前景**：實體 HID 一定移動真實游標，無法背景操作；`Win32-Window` 的純後台操作在此模式不可用。
- **視窗必須可見**：anchor 是遊戲視窗的 client 矩形，視窗被最小化或關掉時 SDK 解不出矩形
  （`Unsupported`），該次點擊直接失敗；`gdbinput` 會在下一輪重新綁定視窗（`gdb_window_metrics`
  也會回報 `minimized`／`visible`）。
- **DPI**：SDK 用實體像素（`GetPhysicalCursorPos`），`gdbinput` 在找視窗前呼叫 SDK 的
  `gdb_platform_prepare()`（等於 per-monitor v2；`gdb_open` 也會做一次），座標換算與落點才一致。
  縮放比例非 100% 時請先用簡單任務驗落點。
- **共用裝置**：與 FirmwareManageTool、MSCV、EvanRemote 併用時最多八個 session，鍵盤/按鈕是位元聯集
  （同鍵要所有 session 放開才放開），位移依接收順序作用在同一支游標。多個程式同時在移動時落點會互相
  打斷，建議跑 MDA 時不要同時做量測。
- **沒有滾輪動作**：目前 pipeline 沒有 `Scroll` 節點；`GdbScroll` 已就緒，`dx`／`dy` 用
  WHEEL_DELTA（120 = 一格），包裝層換算成格數再送（實測 `Wheel(1,0)` → Windows `WM_MOUSEWHEEL
delta=120`，方向不反轉）。
- 反作弊：實體 HID 免驅動、免管理員，但**不保證**不被判定，風險請自行評估。

## 實機踩過的坑（改動這段程式前先看）

- **`gdb.dll` 找不到就整個退回內建輸入**：LoadLibrary 只找 exe 目錄、system32 與 PATH，`agent/`
  子目錄不算。`tools/install.py` 已處理；手動部署時要放在 `go-service.exe` 旁邊。
- **前景鎖**：`SetForegroundWindow` 只在呼叫端是前景行程時有效。本機的
  `SPI_GETFOREGROUNDLOCKTIMEOUT` 被設成 `2147483647`（無限），導致 `Focus()` 第一次就失敗、
  整個任務退回內建輸入（log：`session setup failed ... failed to bring window ... to the foreground`）。
  現在 `Focus()` 會依序試 AttachThreadInput／BringWindowToTop，再把該逾時降成 0（只做一次並記 log
  `foreground lock timeout lowered to 0`），最後才用最小化＋還原。實測 41 ms 成功。
- **SDK 的絕對定位不要用 0（自然時長）跑點擊**：`duration_ms = 0` 會依距離挑「像人」的時長（實測 1 秒
  級）。gdbinput 的點擊固定用 60 ms 線性，實測 83 ms 落地。
- **`click_timed` 的 0 不是「不要等」**：SDK 把 `hold_ms`／`gap_ms` 的 0 解讀成「跟 `click()` 一樣用
  人味分布」，hold 會落在 38–150 ms、gap 落在 60–340 ms。MDA 傳實際的 `click_hold_ms`，gap 固定
  1 ms（見 `device_windows.go` 的 `clickGapMS`）。
- **落點回報要看 `clamped`**：目標落在 anchor 外時 SDK 會夾進矩形並在 `gdb_move_report.clamped`
  標記，MDA 會記 `pointer landed off target`。看到這行先懷疑視窗被拖動或 anchor 不是 client 區。
- 實測值（XIAO RP2040 + 3.2.0）：落點 `error=(0,1)`；60 ms 移動牆鐘約 83 ms。

## 驗證

```powershell
# 需在 agent/go-service 目錄
go test ./gdbinput/
```

- `gdbinput`：JSONC 解析、override 覆蓋整個輸入面（≥400 節點）、Click 語義、VK→HID 對照，以及用
  `gdb_open_mock` 驗證 GDBOX 包裝層（移動／按鈕／拖曳／按鍵／釋放），並釘住 client 區 anchor、
  定時點擊的 hold 換算與 SDK 錯誤附加（`TestWindowClientAnchor`／`TestHoldMS`）。
- SDK 本身的測試在 `FirmwareManageTool_Rust`：`cargo test -p gdb -p gdb-motion`，
  `bash scripts/check-sdk.sh` 會連 C++／Python／Go 綁定一起跑。

實機診斷用 `cmd/gdbprobe`（`install/gdbprobe.exe`，需要 `gdb.dll` 同目錄）：

| 指令                      | 用途                                               |
| ------------------------- | -------------------------------------------------- |
| `gdbprobe`                | 列出 SDK 認得的裝置與能力（board／firmware／caps） |
| `gdbprobe status`         | 開 session 後查詢 STATUS                           |
| `gdbprobe move X Y [ms]`  | 移動游標並印出落點誤差與耗時                       |
| `gdbprobe click X Y [ms]` | 移動並左鍵點擊（會真的點下去）                     |
| `gdbprobe key A`          | 按一次指定鍵                                       |

要看**所有** HID 介面（例如別家的 `0xFF60` 集合、確認列舉是否正常），用 SDK 附的 probe：
在 `FirmwareManageTool_Rust` 目錄執行 `cargo run --release --example probe`。

尚未驗證：實機任務成功率、遊戲端是否接受相對滑鼠、與遊戲解析度／DPI 的組合。
