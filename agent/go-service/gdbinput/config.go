package gdbinput

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Config 是 GDBOX 硬體輸入的設定，存放於使用者設定目錄的 MDA/gdb-input.json。
type Config struct {
	// Enabled："auto"（預設，偵測到裝置就啟用）、"on"、"off"。
	Enabled string `json:"enabled"`
	// ImageWidth / ImageHeight 是 pipeline 使用的辨識影像尺寸（MDA 固定 1280×720）。
	ImageWidth  int32 `json:"image_width"`
	ImageHeight int32 `json:"image_height"`
	// WindowClassRegex / WindowTitleRegex 與 interface.json 的 Win32 控制器一致。
	WindowClassRegex string `json:"window_class_regex"`
	WindowTitleRegex string `json:"window_title_regex"`
	// ClickHoldMS 是按鈕按住的毫秒數。
	ClickHoldMS int `json:"click_hold_ms"`
}

const (
	defaultImageWidth  = 1280
	defaultImageHeight = 720
	defaultClassRegex  = "UnityWndClass"
	defaultTitleRegex  = `(NIKKE|勝利女神：妮姬)`
)

// DefaultConfig 回傳預設值。
func DefaultConfig() Config {
	return Config{
		Enabled:          "auto",
		ImageWidth:       defaultImageWidth,
		ImageHeight:      defaultImageHeight,
		WindowClassRegex: defaultClassRegex,
		WindowTitleRegex: defaultTitleRegex,
		ClickHoldMS:      40,
	}
}

// ConfigPath 回傳設定檔路徑（%APPDATA%\MDA\gdb-input.json）。
func ConfigPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}
	return filepath.Join(dir, "MDA", "gdb-input.json"), nil
}

// LoadConfig 讀取設定；檔案不存在時回傳預設值。
func LoadConfig() Config {
	cfg := DefaultConfig()
	path, err := ConfigPath()
	if err != nil {
		return cfg
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return cfg
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return DefaultConfig()
	}
	cfg.normalize()
	return cfg
}

func (c *Config) normalize() {
	if c.Enabled == "" {
		c.Enabled = "auto"
	}
	c.Enabled = strings.ToLower(strings.TrimSpace(c.Enabled))
	switch c.Enabled {
	case "auto", "on", "off":
	default:
		c.Enabled = "auto"
	}
	if c.ImageWidth <= 0 {
		c.ImageWidth = defaultImageWidth
	}
	if c.ImageHeight <= 0 {
		c.ImageHeight = defaultImageHeight
	}
	if strings.TrimSpace(c.WindowClassRegex) == "" {
		c.WindowClassRegex = defaultClassRegex
	}
	if strings.TrimSpace(c.WindowTitleRegex) == "" {
		c.WindowTitleRegex = defaultTitleRegex
	}
	if c.ClickHoldMS <= 0 {
		c.ClickHoldMS = 40
	}
}

// EnvOverride 讀取環境變數 MDA_GDB_INPUT，回傳設定值或空字串。
func EnvOverride() string {
	return strings.ToLower(strings.TrimSpace(os.Getenv("MDA_GDB_INPUT")))
}

// EnabledWith 依設定與環境變數決定是否啟用；devicePresent 表示是否偵測到裝置。
func (c Config) EnabledWith(devicePresent bool) bool {
	return c.EnabledWithMode(devicePresent, "")
}

// EnabledWithMode 再加上 UI 選項的模式。優先序：環境變數 > UI 選項 > 設定檔 > auto。
func (c Config) EnabledWithMode(devicePresent bool, mode string) bool {
	switch EnvOverride() {
	case "1", "on", "true", "yes":
		return true
	case "0", "off", "false", "no":
		return false
	}
	switch mode {
	case "on":
		return true
	case "off":
		return false
	}
	switch c.Enabled {
	case "on":
		return true
	case "off":
		return false
	default:
		return devicePresent
	}
}
