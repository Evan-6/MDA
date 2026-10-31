//go:build !windows

package gdbinput

import "errors"

// Window 在非 Windows 平台沒有實作。
type Window struct {
	Handle  uintptr
	Title   string
	Class   string
	OriginX int32
	OriginY int32
	ClientW int32
	ClientH int32
}

// FindWindow 在非 Windows 平台一律回傳不支援。
func FindWindow(classPattern, titlePattern string) (*Window, error) {
	return nil, errors.New("window discovery is only available on Windows")
}

// platformFindWindow 在非 Windows 平台一律回傳不支援。
func platformFindWindow(Config) (window, error) {
	return nil, errors.New("window discovery is only available on Windows")
}

// ToScreen 在非 Windows 平台僅做原點平移。
func (w *Window) ToScreen(imgX, imgY, imgW, imgH int32) (int32, int32) {
	return w.OriginX + imgX, w.OriginY + imgY
}

// Screen 在非 Windows 平台回傳主螢幕（0）；硬體輸入本身也不可用。
func (w *Window) Screen() screenTarget { return screenTarget{} }

// IsForeground 在非 Windows 平台一律回傳 false。
func (w *Window) IsForeground() bool { return false }

// Focus 在非 Windows 平台一律回傳不支援。
func (w *Window) Focus() error {
	return errors.New("focusing a window is only available on Windows")
}
