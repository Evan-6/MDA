//go:build windows

package gdbinput

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	gdb "github.com/nousresearch/gdbox-sdk"
)

// TestGdbDeviceAgainstMock 用 SDK 的模擬裝置驗證 GDBOX 包裝層的接線：
// 移動 → 按鈕 → 拖曳 → 按鍵 → 釋放，全部走真實的 gdb.dll ABI，不需要硬體。
func TestGdbDeviceAgainstMock(t *testing.T) {
	dll, err := filepath.Abs(filepath.Join("..", "gdbox-sdk", "target", "release", "gdb.dll"))
	if err != nil {
		t.Fatalf("resolve dll: %v", err)
	}
	if _, err := os.Stat(dll); err != nil {
		t.Skipf("gdb.dll 未編譯（python tools/gdbox.py）: %v", err)
	}
	t.Setenv("GDBOX_LIBRARY", dll)

	dev, err := gdb.OpenMock()
	if err != nil {
		t.Fatalf("open mock: %v", err)
	}
	wrapped := &gdbDevice{dev: dev}
	defer func() { _ = wrapped.Close() }()

	if err := wrapped.Click(screenTarget{}, 120, 220, 10*time.Millisecond); err != nil {
		t.Fatalf("click: %v", err)
	}
	if err := wrapped.Drag(screenTarget{}, 120, 220, 320, 260, 40*time.Millisecond, 5*time.Millisecond); err != nil {
		t.Fatalf("drag: %v", err)
	}
	if err := wrapped.Key(7, 0x04, true); err != nil {
		t.Fatalf("key down: %v", err)
	}
	if err := wrapped.Key(7, 0x04, false); err != nil {
		t.Fatalf("key up: %v", err)
	}
	if err := wrapped.TapKey(7, 0x05, 10*time.Millisecond); err != nil {
		t.Fatalf("tap key: %v", err)
	}
	if err := wrapped.ReleaseAll(); err != nil {
		t.Fatalf("release all: %v", err)
	}

	// SDK 的 device 級錯誤要能被包裝層補進訊息，任務日誌才看得到失敗的 opcode。
	wrappedErr := wrapped.wrap("probe", os.ErrDeadlineExceeded)
	if !errors.Is(wrappedErr, os.ErrDeadlineExceeded) {
		t.Fatalf("wrap must keep the original error: %v", wrappedErr)
	}
}

// TestWindowClientAnchor 釘住「遊戲視窗用 client 區當 anchor」。
//
// 外框包含標題列與 Win10/11 的隱形 resize 邊框，貼邊的目標會被夾到邊框裡；
// 這個測試紅掉代表有人把 Screen() 改回 kind 2（外框）。
func TestWindowClientAnchor(t *testing.T) {
	win := &Window{Handle: 0x1234}
	at := win.Screen()
	if at.Kind != screenKindWindowClient {
		t.Fatalf("window anchor kind = %d, want %d (window client)", at.Kind, screenKindWindowClient)
	}
	if at.Arg != 0x1234 {
		t.Fatalf("window anchor handle = %#x, want 0x1234", at.Arg)
	}
	if gdb.ScreenWindowClient(0x1234).Kind != screenKindWindowClient {
		t.Fatal("screenKindWindowClient must match the SDK's ScreenWindowClient")
	}
}

// TestHoldMS 釘住「0 代表不要按住」的換算（SDK 的 0 是人味預設長度）。
func TestHoldMS(t *testing.T) {
	cases := []struct {
		hold time.Duration
		want uint32
	}{
		{0, 1},
		{time.Millisecond, 1},
		{40 * time.Millisecond, 40},
		{1500 * time.Millisecond, 1500},
	}
	for _, c := range cases {
		if got := holdMS(c.hold); got != c.want {
			t.Fatalf("holdMS(%v) = %d, want %d", c.hold, got, c.want)
		}
	}
}

// TestClickHonoursScreenTarget 確認 Click／Drag 把夾取矩形真的交給 SDK。
//
// 模擬裝置沒有視窗，所以「視窗 anchor」必定失敗；若包裝層忽略 anchor 走主螢幕
// （舊行為），這兩個呼叫會成功，測試就紅。這正是副螢幕視窗被拉回主螢幕的成因。
func TestClickHonoursScreenTarget(t *testing.T) {
	dll, err := filepath.Abs(filepath.Join("..", "gdbox-sdk", "target", "release", "gdb.dll"))
	if err != nil {
		t.Fatalf("resolve dll: %v", err)
	}
	if _, err := os.Stat(dll); err != nil {
		t.Skipf("gdb.dll 未編譯（python tools/gdbox.py）: %v", err)
	}
	t.Setenv("GDBOX_LIBRARY", dll)

	dev, err := gdb.OpenMock()
	if err != nil {
		t.Fatalf("open mock: %v", err)
	}
	wrapped := &gdbDevice{dev: dev}
	defer func() { _ = wrapped.Close() }()

	anchor := screenTarget{Kind: 2, Arg: 0x1234}
	if err := wrapped.Click(anchor, 300, 200, 0); err == nil {
		t.Fatal("click with a window anchor should fail on a mock device (no windows)")
	}
	if err := wrapped.Drag(anchor, 300, 200, 400, 260, 20*time.Millisecond, 0); err == nil {
		t.Fatal("drag with a window anchor should fail on a mock device (no windows)")
	}
}

// TestPressAndWheelAgainstMock 驗新動作的 SDK 接線：按鍵位元、滾輪切段。
func TestPressAndWheelAgainstMock(t *testing.T) {
	dll, err := filepath.Abs(filepath.Join("..", "gdbox-sdk", "target", "release", "gdb.dll"))
	if err != nil {
		t.Fatalf("resolve dll: %v", err)
	}
	if _, err := os.Stat(dll); err != nil {
		t.Skipf("gdb.dll 未編譯（python tools/gdbox.py）: %v", err)
	}
	t.Setenv("GDBOX_LIBRARY", dll)

	dev, err := gdb.OpenMock()
	if err != nil {
		t.Fatalf("open mock: %v", err)
	}
	wrapped := &gdbDevice{dev: dev}
	defer func() { _ = wrapped.Close() }()

	// 三個觸點都要能按（0 左、1 右、2 中）；3 是 XBUTTON，實體游標做不到。
	for _, contact := range []uint8{0, 1, 2} {
		if err := wrapped.Press(screenTarget{}, contact, 100, 120, 5*time.Millisecond); err != nil {
			t.Fatalf("press contact %d: %v", contact, err)
		}
	}
	if err := wrapped.Press(screenTarget{}, 3, 100, 120, 5*time.Millisecond); err == nil {
		t.Fatal("contact 3 must be rejected")
	}

	// SDK 對單包 wheel 擋在 ±1024；超過還成功就代表包裝層有切段。
	if err := wrapped.Wheel(screenTarget{}, 100, 120, 120, 0); err != nil {
		t.Fatalf("wheel one notch: %v", err)
	}
	if err := wrapped.Wheel(screenTarget{}, 100, 120, -3600, 2400); err != nil {
		t.Fatalf("wheel beyond one packet (±1024) should be chunked: %v", err)
	}
	if err := wrapped.Wheel(screenTarget{}, 100, 120, 0, 0); err != nil {
		t.Fatalf("wheel with nothing to send: %v", err)
	}
}

// TestWheelNotches 釘住 WHEEL_DELTA → 格數的換算（實機量的：1 格 = Windows delta 120）。
func TestWheelNotches(t *testing.T) {
	cases := []struct{ delta, want int32 }{
		{0, 0},
		{120, 1},
		{-120, -1},
		{240, 2},
		{-360, -3},
		{2400, 20},
		// 不足一格也要滾一格，否則 pipeline 寫 dy=60 會靜默不滾。
		{60, 1},
		{-60, -1},
		{119, 1},
		{-119, -1},
	}
	for _, c := range cases {
		if got := wheelNotches(c.delta); got != c.want {
			t.Fatalf("wheelNotches(%d) = %d, want %d", c.delta, got, c.want)
		}
	}
}

// TestHardwareWheelManual 走完整包裝層（換算＋切段）對真裝置滾一格，只在
// GDBINPUT_HARDWARE=1 時跑；它會移動真實游標，實機驗證時配合系統滾輪事件觀察
// （實測 Windows 收到 WM_MOUSEWHEEL delta=120）。
func TestHardwareWheelManual(t *testing.T) {
	if os.Getenv("GDBINPUT_HARDWARE") != "1" {
		t.Skip("set GDBINPUT_HARDWARE=1 to drive the physical device")
	}
	dev, err := gdb.Open("", 1000, true, true)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	wrapped := &gdbDevice{dev: dev}
	defer func() { _ = wrapped.Close() }()

	if err := wrapped.Wheel(screenTarget{}, 600, 450, wheelDelta, 0); err != nil {
		t.Fatalf("wheel: %v", err)
	}
}

// TestGuardSDKConvertsPanic 確認 gdb.dll 不在搜尋路徑時（LazyProc.Call panic）
// 會被轉成錯誤，而不是讓 agent 崩掉。
func TestGuardSDKConvertsPanic(t *testing.T) {
	err := guardSDK(func() error { panic("Failed to load gdb.dll") })
	if err == nil {
		t.Fatal("panic should become an error")
	}
	if err := guardSDK(func() error { return nil }); err != nil {
		t.Fatalf("normal call returned %v", err)
	}
}
