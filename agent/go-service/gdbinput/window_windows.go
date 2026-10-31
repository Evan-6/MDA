//go:build windows

package gdbinput

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"

	gdb "github.com/nousresearch/gdbox-sdk"
)

var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")

	procEnumWindows      = user32.NewProc("EnumWindows")
	procGetClassNameW    = user32.NewProc("GetClassNameW")
	procGetWindowTextW   = user32.NewProc("GetWindowTextW")
	procIsWindowVisible  = user32.NewProc("IsWindowVisible")
	procGetForegroundWnd = user32.NewProc("GetForegroundWindow")
	procSetForegroundWnd = user32.NewProc("SetForegroundWindow")
	procShowWindow       = user32.NewProc("ShowWindow")
	procIsIconic         = user32.NewProc("IsIconic")

	// 前景鎖：SetForegroundWindow 只在呼叫端是前景行程時有效，要先拿到權限。
	procAttachThreadInput        = user32.NewProc("AttachThreadInput")
	procGetWindowThreadProcessID = user32.NewProc("GetWindowThreadProcessId")
	procGetCurrentThreadID       = kernel32.NewProc("GetCurrentThreadId")
	procBringWindowToTop         = user32.NewProc("BringWindowToTop")
	procSetWindowPos             = user32.NewProc("SetWindowPos")
	procSystemParamsInfo         = user32.NewProc("SystemParametersInfoW")
)

// Window 是目標遊戲視窗的客戶區資訊。
//
// 客戶區原點與大小由 SDK 的 WindowMetrics 提供：它與絕對移動共用同一套
// per-monitor v2 DPI 與實體像素規則，所以影像座標換算不會和 SDK 的落點
// 差一個縮放倍率（這裡不再自己呼叫 ClientToScreen）。
type Window struct {
	Handle  uintptr
	Title   string
	Class   string
	OriginX int32
	OriginY int32
	ClientW int32
	ClientH int32
	DPI     uint32
}

var platformPrepareOnce sync.Once

// platformFindWindow 找出遊戲視窗並記錄綁定資訊。
//
// 先叫 SDK 準備平台（per-monitor v2 DPI awareness）：SDK 的絕對定位用實體像素
// （GetPhysicalCursorPos），WindowMetrics 也走同一組座標，兩邊才會一致。
func platformFindWindow(cfg Config) (window, error) {
	preparePlatform()
	win, err := FindWindow(cfg.WindowClassRegex, cfg.WindowTitleRegex)
	if err != nil {
		return nil, err
	}
	logger.Info().
		Str("title", win.Title).
		Str("class", win.Class).
		Int32("client_w", win.ClientW).
		Int32("client_h", win.ClientH).
		Int32("origin_x", win.OriginX).
		Int32("origin_y", win.OriginY).
		Uint32("dpi", win.DPI).
		Msg("game window bound")
	return win, nil
}

// preparePlatform 讓 SDK 把行程切成 per-monitor v2 並回報目前狀態（可重複呼叫）。
func preparePlatform() {
	platformPrepareOnce.Do(func() {
		state, err := gdb.PreparePlatform()
		if err != nil {
			logger.Warn().Err(err).Msg("platform prepare failed")
			return
		}
		logger.Info().
			Bool("supported", state.Supported).
			Str("dpi_awareness", state.DpiAwareness).
			Msg("platform prepared for absolute moves")
	})
}

// FindWindow 依 class／title 正則找出第一個可見的符合視窗。
func FindWindow(classPattern, titlePattern string) (*Window, error) {
	classRe, err := regexp.Compile(classPattern)
	if err != nil {
		return nil, fmt.Errorf("invalid window_class_regex: %w", err)
	}
	titleRe, err := regexp.Compile(titlePattern)
	if err != nil {
		return nil, fmt.Errorf("invalid window_title_regex: %w", err)
	}

	var found *Window
	callback := windows.NewCallback(func(hwnd uintptr, _ uintptr) uintptr {
		if visible, _, _ := procIsWindowVisible.Call(hwnd); visible == 0 {
			return 1
		}
		class := windowText(procGetClassNameW, hwnd, 256)
		if !classRe.MatchString(class) {
			return 1
		}
		title := windowText(procGetWindowTextW, hwnd, 512)
		if !titleRe.MatchString(title) {
			return 1
		}
		metrics, err := gdb.WindowMetrics(int64(hwnd))
		if err != nil || metrics.Minimized || !metrics.Visible {
			return 1
		}
		if metrics.ClientSize.Width <= 0 || metrics.ClientSize.Height <= 0 {
			return 1
		}
		found = &Window{
			Handle:  hwnd,
			Title:   title,
			Class:   class,
			OriginX: metrics.ClientOrigin.X,
			OriginY: metrics.ClientOrigin.Y,
			ClientW: metrics.ClientSize.Width,
			ClientH: metrics.ClientSize.Height,
			DPI:     metrics.Dpi,
		}
		return 0 // 找到就停止列舉
	})
	procEnumWindows.Call(callback, 0)
	if found == nil {
		return nil, fmt.Errorf("no visible window matched class %q title %q", classPattern, titlePattern)
	}
	return found, nil
}

func windowText(proc *windows.LazyProc, hwnd uintptr, size int) string {
	buffer := make([]uint16, size)
	if n, _, _ := proc.Call(hwnd, uintptr(unsafe.Pointer(&buffer[0])), uintptr(size)); n == 0 {
		return ""
	}
	return strings.TrimSpace(windows.UTF16ToString(buffer))
}

// ToScreen 把辨識影像座標換算成螢幕座標。
func (w *Window) ToScreen(imgX, imgY, imgW, imgH int32) (int32, int32) {
	if imgW <= 0 || imgH <= 0 {
		return w.OriginX + imgX, w.OriginY + imgY
	}
	x := w.OriginX + int32(float64(imgX)*float64(w.ClientW)/float64(imgW))
	y := w.OriginY + int32(float64(imgY)*float64(w.ClientH)/float64(imgH))
	return x, y
}

// Screen 回傳視窗 client 區，供絕對移動當夾取範圍。
//
// 用 client 區而不是外框：外框包含標題列與 Win10/11 的隱形 resize 邊框，
// 貼邊的目標會被夾到看不見的邊框上。視窗在副螢幕或負座標時 SDK 也照樣
// 落在同一個 client 座標系。
func (w *Window) Screen() screenTarget {
	return screenTarget{Kind: screenKindWindowClient, Arg: int64(w.Handle)}
}

// IsForeground 回報此視窗是否為前景視窗。
func (w *Window) IsForeground() bool {
	hwnd, _, _ := procGetForegroundWnd.Call()
	return hwnd == w.Handle
}

// Focus 把目標視窗帶到前景；HID 輸入只會作用在前景視窗。
//
// Windows 有前景鎖：非前景行程直接呼叫 SetForegroundWindow 會被忽略（實測在 agent 裡第一次
// 就失敗，整個任務因此退回內建輸入）。這裡依序用三種手法，任何一種成功就回傳：
//  1. AttachThreadInput 掛到目前前景執行緒，借用它的前景權限。
//  2. 模擬 ALT 鍵（系統會允許緊接的前景切換）。
//  3. 最小化後還原，讓視窗自己取得前景。
func (w *Window) Focus() error {
	if w.IsForeground() {
		return nil
	}
	const swRestore, swMinimize = 9, 6
	if iconic, _, _ := procIsIconic.Call(w.Handle); iconic != 0 {
		procShowWindow.Call(w.Handle, swRestore)
	}

	// 1) 借用前景執行緒權限
	foreground, _, _ := procGetForegroundWnd.Call()
	currentThread, _, _ := procGetCurrentThreadID.Call()
	var foregroundThread uintptr
	attached := false
	if foreground != 0 {
		foregroundThread, _, _ = procGetWindowThreadProcessID.Call(foreground, 0)
		if foregroundThread != 0 && foregroundThread != currentThread {
			if ok, _, _ := procAttachThreadInput.Call(currentThread, foregroundThread, 1); ok != 0 {
				attached = true
			}
		}
	}
	procBringWindowToTop.Call(w.Handle)
	const swpNoSize, swpNoMove, hwndTop = 0x0001, 0x0002, 0
	procSetWindowPos.Call(w.Handle, hwndTop, 0, 0, 0, 0, swpNoSize|swpNoMove)
	procSetForegroundWnd.Call(w.Handle)
	if attached {
		procAttachThreadInput.Call(currentThread, foregroundThread, 0)
	}
	if w.IsForeground() {
		return nil
	}

	// 2) 前景鎖逾時：這台機器的值被設成 2147483647（無限），上面那些手法因此全被忽略。
	//    改成 0 之後 SetForegroundWindow 才會生效；立即還原會讓接下來的切換又被擋掉，
	//    所以只改一次並記錄下來（0 是遊戲環境常見值，不影響其他程式）。
	ensureForegroundLockDisabled()
	procSetForegroundWnd.Call(w.Handle)
	if w.IsForeground() {
		return nil
	}

	// 3) 最小化再還原
	procShowWindow.Call(w.Handle, swMinimize)
	procShowWindow.Call(w.Handle, swRestore)
	procSetForegroundWnd.Call(w.Handle)
	if w.IsForeground() {
		return nil
	}
	return fmt.Errorf("failed to bring window %q to the foreground", w.Title)
}

const (
	spiGetForegroundLockTimeout = 0x2000
	spiSetForegroundLockTimeout = 0x2001
	spifUpdateIniFile           = 0x1
	spifSendChange              = 0x2
)

var foregroundLockOnce sync.Once

// ensureForegroundLockDisabled 把前景鎖逾時降成 0（只做一次，並記錄原始值）。
func ensureForegroundLockDisabled() {
	foregroundLockOnce.Do(func() {
		previous := foregroundLockTimeout()
		if previous == 0 {
			return
		}
		setForegroundLockTimeout(0)
		logger.Info().Uint32("previous_ms", previous).
			Msg("foreground lock timeout lowered to 0 so the game window can be focused")
	})
}

// foregroundLockTimeout 讀回目前的前景鎖逾時（毫秒）。0 表示沒有鎖。
func foregroundLockTimeout() uint32 {
	var value uint32
	if ok, _, _ := procSystemParamsInfo.Call(
		spiGetForegroundLockTimeout, 0, uintptr(unsafe.Pointer(&value)), 0); ok == 0 {
		return 0
	}
	return value
}

// setForegroundLockTimeout 設定前景鎖逾時並立即生效。
func setForegroundLockTimeout(value uint32) {
	procSystemParamsInfo.Call(spiSetForegroundLockTimeout, 0, uintptr(value),
		spifUpdateIniFile|spifSendChange)
}
