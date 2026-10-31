//go:build windows

package gdbinput

import (
	"encoding/json"
	"fmt"
	"time"

	gdb "github.com/nousresearch/gdbox-sdk"
)

const (
	// openTimeoutMS 是 OPEN 等裝置回覆的上限（SDK 下限 100 ms）。
	openTimeoutMS = 1000
	// clickMoveMS 是點擊前移動游標的時長；SDK 的閉環會自己收斂落點。
	clickMoveMS = 60
	// clickGapMS 是定時點擊放開後等待的時間。SDK 把 0 解讀成「跟 click() 一樣用
	// 人味間隔」（60–340 ms），單次點擊不需要，所以固定 1 ms。
	clickGapMS = 1
	// landingTolerancePx 是落點誤差超過就記警告的門檻（實體游標誤差約 1 px）。
	landingTolerancePx = 2
	// wheelDelta 是 Windows 的一格滾輪（WHEEL_DELTA）；pipeline 的 dx／dy 用它當單位。
	wheelDelta = 120
	// wheelLimit 是韌體單包 WHEEL 的格數上限（i16，SDK 也擋在 ±1024）。
	wheelLimit = 1024
)

// gdbDevice 是本套件對 GDBOX SDK 的薄包裝：只把 pipeline 語義
// （按住多久、拖多久、按哪個鍵）翻成 SDK 呼叫，協議與 session 都交給 SDK。
type gdbDevice struct {
	dev *gdb.Device
}

// raw 把平台中立的 screenTarget 轉成 SDK 的 gdb.Screen。
func (s screenTarget) raw() gdb.Screen { return gdb.Screen{Kind: s.Kind, Arg: s.Arg} }

// guardSDK 把「SDK 載入失敗」轉成錯誤。
//
// syscall.LazyDLL 找不到 gdb.dll 或其匯出函式時是 **panic**，不是 error；
// 少了這層，部署時漏掉 DLL 會讓 agent 整個崩掉，而不是照設計退回內建輸入。
func guardSDK(fn func() error) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("GDBOX SDK unavailable (%v)；請確認 gdb.dll 與 go-service.exe 同目錄", recovered)
		}
	}()
	return fn()
}

// listGDBOXDevices 回傳 SDK 認得的裝置數量。
func listGDBOXDevices() (int, error) {
	count := 0
	err := guardSDK(func() error {
		raw, err := gdb.ListJSON()
		if err != nil {
			return err
		}
		var entries []struct {
			Path   string `json:"path"`
			Serial string `json:"serial"`
		}
		if err := json.Unmarshal([]byte(raw), &entries); err != nil {
			return fmt.Errorf("device list is not JSON: %w", err)
		}
		count = len(entries)
		return nil
	})
	return count, err
}

// platformDevicePresent 回報目前是否有 SDK 認得的裝置（GDBOX 韌體、VID:PID 03F0:0024）。
func platformDevicePresent() bool {
	count, err := listGDBOXDevices()
	if err != nil {
		logger.Warn().Err(err).Msg("device scan failed")
		return false
	}
	return count > 0
}

// platformOpenDevice 開一個 SDK session（心跳、租約、單次自動重連都在 SDK 內）。
func platformOpenDevice(Config) (device, error) {
	var dev *gdb.Device
	err := guardSDK(func() error {
		opened, err := gdb.Open("", openTimeoutMS, true, true)
		if err != nil {
			return fmt.Errorf("open GDBOX device: %w", err)
		}
		dev = opened
		major, minor := gdb.LibraryABIVersions()
		capabilities, _ := gdb.Capabilities()
		logger.Info().
			Uint32("abi_major", major).
			Uint32("abi_minor", minor).
			Str("capabilities", capabilities).
			Msg("GDBOX SDK ready")
		return nil
	})
	if err != nil {
		return nil, err
	}
	if info, err := dev.InfoJSON(); err == nil {
		logger.Info().Str("info", info).Msg("GDBOX session opened")
	} else {
		logger.Info().Msg("GDBOX session opened")
	}
	return &gdbDevice{dev: dev}, nil
}

// wrap 在 SDK 錯誤後面補上 device 級錯誤（含失敗的 opcode），讓任務日誌
// 不必只靠一行 "code 5" 猜測是哪一個指令失敗。
func (d *gdbDevice) wrap(op string, err error) error {
	if err == nil {
		return nil
	}
	code, detail := d.dev.LastError()
	if detail == "" {
		return fmt.Errorf("%s: %w", op, err)
	}
	return fmt.Errorf("%s: %w (SDK error %d: %s)", op, err, int32(code), detail)
}

// holdMS 把 pipeline 的按住時間換成 SDK 的毫秒。
//
// 0 用 1 ms 而不是交給 SDK：SDK 的 0 代表「用 click() 的人味長度」（38–150 ms），
// pipeline 寫 0 的意思是「不要按住」。
func holdMS(hold time.Duration) uint32 {
	if ms := uint32(hold / time.Millisecond); ms > 0 {
		return ms
	}
	return 1
}

// moveTo 把游標移到 (x,y)，並在 SDK 回報被夾住或落點誤差過大時記一筆警告。
//
// MoveToOnReport 是 SDK 唯一會回報實際落點的路徑：clamp 代表目標落在 anchor
// 之外（視窗被拖走、視窗在別的螢幕），error_px 代表閉環沒有收斂。
func (d *gdbDevice) moveTo(op string, at screenTarget, x, y int32, durationMs uint32) error {
	profile := gdb.LinearProfile()
	report, err := d.dev.Mouse().MoveToOnReport(x, y, at.raw(), durationMs, &profile)
	if err != nil {
		return d.wrap(op+" move", err)
	}
	if report.Clamped || report.ErrorPx() > landingTolerancePx {
		logger.Warn().
			Str("op", op).
			Int32("requested_x", report.RequestedX).
			Int32("requested_y", report.RequestedY).
			Int32("applied_x", report.AppliedX).
			Int32("applied_y", report.AppliedY).
			Bool("clamped", report.Clamped).
			Int32("error_px", report.ErrorPx()).
			Msg("pointer landed off target")
	}
	return nil
}

// Click 移動到螢幕座標後用 SDK 的定時點擊按住左鍵 hold 再放開，最後等 USB 完成。
func (d *gdbDevice) Click(at screenTarget, x, y int32, hold time.Duration) error {
	return guardSDK(func() error {
		if err := d.moveTo("click", at, x, y, clickMoveMS); err != nil {
			return err
		}
		if err := d.dev.Mouse().ClickTimed(gdb.Left, 1, holdMS(hold), clickGapMS, 0); err != nil {
			return d.wrap("click", err)
		}
		return d.dev.Barrier()
	})
}

// Drag 從 (x1,y1) 按住左鍵沿直線拖到 (x2,y2)，可在終點停留 hold。
func (d *gdbDevice) Drag(at screenTarget, x1, y1, x2, y2 int32, duration, hold time.Duration) error {
	return guardSDK(func() error {
		if err := d.moveTo("drag start", at, x1, y1, clickMoveMS); err != nil {
			return err
		}
		if err := d.dev.Mouse().Button(gdb.Left, true); err != nil {
			return d.wrap("drag press", err)
		}
		ms := uint32(duration / time.Millisecond)
		if ms == 0 {
			ms = 400
		}
		if err := d.moveTo("drag end", at, x2, y2, ms); err != nil {
			_ = d.dev.Mouse().Button(gdb.Left, false)
			return err
		}
		if hold > 0 {
			time.Sleep(hold)
		}
		if err := d.dev.Mouse().Button(gdb.Left, false); err != nil {
			return d.wrap("drag release", err)
		}
		return d.dev.Barrier()
	})
}

func (d *gdbDevice) Key(page, usage uint16, down bool) error {
	return guardSDK(func() error {
		if down {
			return d.wrap("key down", d.dev.Keyboard().KeyDown(page, usage))
		}
		return d.wrap("key up", d.dev.Keyboard().KeyUp(page, usage))
	})
}

func (d *gdbDevice) TapKey(page, usage uint16, hold time.Duration) error {
	return guardSDK(func() error {
		ms := uint32(hold / time.Millisecond)
		if ms == 0 {
			ms = 40
		}
		return d.wrap("key press", d.dev.Keyboard().LongPress(page, usage, ms))
	})
}

// Press 移動到 (x,y) 後用定時點擊按住指定滑鼠鍵 hold 再放開。
func (d *gdbDevice) Press(at screenTarget, button uint8, x, y int32, hold time.Duration) error {
	return guardSDK(func() error {
		b, err := mouseButton(button)
		if err != nil {
			return err
		}
		if err := d.moveTo("press", at, x, y, clickMoveMS); err != nil {
			return err
		}
		if err := d.dev.Mouse().ClickTimed(b, 1, holdMS(hold), clickGapMS, 0); err != nil {
			return d.wrap("press", err)
		}
		return d.dev.Barrier()
	})
}

// Wheel 先把游標移到 (x,y)，再滾動。
//
// 傳進來的是 pipeline 的 WHEEL_DELTA 單位（120 = 一格），SDK 要的是「格數」：
// 實測 `Wheel(1, 0)` 讓 Windows 收到 `WM_MOUSEWHEEL delta=120`，`Wheel(0, -1)`
// 收到 `WM_MOUSEHWHEEL delta=-120`。所以這裡換算後才送出，方向不變（dy 正 =
// 向上，dx 正 = 向右）。
func (d *gdbDevice) Wheel(at screenTarget, x, y int32, vertical, horizontal int32) error {
	return guardSDK(func() error {
		v, h := wheelNotches(vertical), wheelNotches(horizontal)
		if v == 0 && h == 0 {
			return nil
		}
		if err := d.moveTo("scroll", at, x, y, clickMoveMS); err != nil {
			return err
		}
		for v != 0 || h != 0 {
			cv, ch := wheelChunk(v), wheelChunk(h)
			if err := d.dev.Mouse().Wheel(cv, ch); err != nil {
				return d.wrap("wheel", err)
			}
			v -= cv
			h -= ch
		}
		return d.dev.Barrier()
	})
}

// wheelNotches 把 WHEEL_DELTA 單位換成格數；不足一格按一格算，避免 ±60 靜默不滾。
func wheelNotches(delta int32) int32 {
	if delta == 0 {
		return 0
	}
	notches := delta / wheelDelta
	if notches == 0 {
		if delta > 0 {
			return 1
		}
		return -1
	}
	return notches
}

// wheelChunk 回傳這一包要送的量：單軸上限 1024 格，方向不變。
func wheelChunk(value int32) int32 {
	switch {
	case value > wheelLimit:
		return wheelLimit
	case value < -wheelLimit:
		return -wheelLimit
	default:
		return value
	}
}

// mouseButton 把 pipeline 的 contact（0 左、1 右、2 中）換成 SDK 的按鍵。
func mouseButton(contact uint8) (gdb.Button, error) {
	switch contact {
	case 0:
		return gdb.Left, nil
	case 1:
		return gdb.Right, nil
	case 2:
		return gdb.Middle, nil
	}
	return 0, fmt.Errorf("contact %d is not a mouse button on a physical cursor", contact)
}

func (d *gdbDevice) ReleaseAll() error { return guardSDK(d.dev.ReleaseAll) }

func (d *gdbDevice) Close() error { return guardSDK(d.dev.Close) }
