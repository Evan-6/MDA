// gdbinput 是硬體輸入的 pipeline 覆寫層。
//
// 裝置通訊完全交給 GDBOX SDK（agent/go-service/gdbox-sdk，執行期載入 gdb.dll）：
// 列舉、session 與租約、心跳、重連、絕對定位閉環都在 SDK 裡。這個套件只保留
// MDA 自己的部分——pipeline 動作改寫、影像座標換算、遊戲視窗前景綁定。
//
// 平台相依的部分由 device_windows.go / device_other.go 提供，因此本套件在
// 非 Windows 平台仍可編譯（CI 會跑 go vet ./...）。
package gdbinput

import "time"

// screenTarget 是絕對移動要夾進的矩形，欄位對應 SDK 的 gdb.Screen：
// Kind 0 = 主螢幕、1 = 螢幕索引、2 = 視窗外框、3 = 包含某點的螢幕、
// 4 = 視窗 client 區（Arg 是視窗 handle）。座標本身不變，仍是虛擬桌面的
// 實體像素；這裡只決定 SDK 把落點夾進哪個矩形。遊戲內點擊一律用 4，
// 外框包含標題列與隱形邊框，夾在外框上會讓貼邊的目標落到邊框裡。
type screenTarget struct {
	Kind int32
	Arg  int64
}

// screenKindWindowClient 是 SDK 的視窗 client 區 anchor（gdb.ScreenWindowClient）。
const screenKindWindowClient = 4

// device 是本套件需要的硬體輸入面；Windows 實作包住 *gdb.Device。
type device interface {
	// Click 把游標移到螢幕座標 (x,y)，用 SDK 的定時點擊按住左鍵 hold 後放開，
	// 並等 USB 完成。at 指定落點要夾進的矩形；遊戲視窗在副螢幕時必須是視窗
	// client 區，傳零值等於夾進主螢幕（視窗不在主螢幕就會被拉偏）。
	Click(at screenTarget, x, y int32, hold time.Duration) error
	// Drag 從 (x1,y1) 按住左鍵拖到 (x2,y2)，可在終點停留 hold。
	Drag(at screenTarget, x1, y1, x2, y2 int32, duration, hold time.Duration) error
	// Press 移動到 (x,y)，按住 button（0 左、1 右、2 中）hold 後放開。
	Press(at screenTarget, button uint8, x, y int32, hold time.Duration) error
	// Wheel 移動到 (x,y) 再滾動；單位是 WHEEL_DELTA（120 = 一格），
	// 正值 vertical 向上、horizontal 向右。
	Wheel(at screenTarget, x, y int32, vertical, horizontal int32) error
	// Key 按下或放開一個 HID 鍵（page + usage）。
	Key(page, usage uint16, down bool) error
	// TapKey 按下並放開一個 HID 鍵。
	TapKey(page, usage uint16, hold time.Duration) error
	// ReleaseAll 放開本 session 的所有按鍵與滑鼠按鈕。
	ReleaseAll() error
	// Close 結束 session 並釋放裝置。
	Close() error
}

// window 是目標遊戲視窗在本套件內的抽象。
type window interface {
	// ToScreen 把辨識影像座標換算成螢幕座標。
	ToScreen(imgX, imgY, imgW, imgH int32) (int32, int32)
	// Screen 回傳這個視窗所在的矩形，供絕對移動當夾取範圍。
	Screen() screenTarget
	// Focus 把視窗帶到前景；實體 HID 只會作用在前景視窗。
	Focus() error
}
