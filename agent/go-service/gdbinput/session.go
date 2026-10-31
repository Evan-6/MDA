package gdbinput

import (
	"sync"
	"time"
)

const windowRefresh = 5 * time.Second

var (
	sessionMu      sync.Mutex
	activeDevice   device
	activeWindow   window
	activeWindowAt time.Time
	warnedOnce     sync.Once
)

// ensureSession 取得（或建立）SDK session 與目標視窗。
//
// 租約、心跳、重連都由 GDBOX SDK 管理；這裡只負責持有它，並在視窗綁定失敗
// 或裝置消失時決定要不要重建。
func ensureSession(cfg Config) (device, window, error) {
	sessionMu.Lock()
	defer sessionMu.Unlock()

	if activeDevice != nil {
		win, err := ensureWindow(cfg)
		if err == nil {
			return activeDevice, win, nil
		}
		logger.Warn().Err(err).Msg("dropping hardware session")
		_ = activeDevice.Close()
		activeDevice = nil
		activeWindow = nil
	}

	dev, err := platformOpenDevice(cfg)
	if err != nil {
		return nil, nil, err
	}

	win, err := ensureWindow(cfg)
	if err != nil {
		_ = dev.Close()
		return nil, nil, err
	}
	// 清掉前一個 session 可能留下的按鍵/按鈕狀態。
	if err := dev.ReleaseAll(); err != nil {
		logger.Warn().Err(err).Msg("release on session start failed")
	}
	activeDevice = dev
	return dev, win, nil
}

func ensureWindow(cfg Config) (window, error) {
	if activeWindow != nil && time.Since(activeWindowAt) < windowRefresh {
		if err := activeWindow.Focus(); err != nil {
			return nil, err
		}
		return activeWindow, nil
	}
	win, err := platformFindWindow(cfg)
	if err != nil {
		return nil, err
	}
	if err := win.Focus(); err != nil {
		return nil, err
	}
	activeWindow = win
	activeWindowAt = time.Now()
	return win, nil
}

// CloseSession 釋放按鍵與裝置（任務結束、agent 結束時呼叫）。
func CloseSession() {
	sessionMu.Lock()
	defer sessionMu.Unlock()
	if activeDevice == nil {
		return
	}
	if err := activeDevice.ReleaseAll(); err != nil {
		logger.Warn().Err(err).Msg("release on close failed")
	}
	if err := activeDevice.Close(); err != nil {
		logger.Warn().Err(err).Msg("close failed")
	}
	activeDevice = nil
	activeWindow = nil
}

// currentSession 取得目前 session，供自訂動作使用。
func currentSession() (device, window, error) {
	cfg := LoadConfig()
	dev, win, err := ensureSession(cfg)
	if err == nil {
		return dev, win, nil
	}
	warnedOnce.Do(func() {
		logger.Warn().Err(err).Msg("hardware input unavailable, task cannot run in hardware mode")
	})
	return nil, nil, err
}
