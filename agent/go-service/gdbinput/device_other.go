//go:build !windows

package gdbinput

import (
	"errors"
)

var errUnsupported = errors.New("hardware input needs Windows and gdb.dll")

// platformDevicePresent 在非 Windows 平台一律回報沒有裝置。
func platformDevicePresent() bool { return false }

// platformOpenDevice 在非 Windows 平台一律回傳不支援。
func platformOpenDevice(Config) (device, error) {
	logger.Debug().Msg("hardware input is only implemented on Windows")
	return nil, errUnsupported
}
