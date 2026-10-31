//go:build windows

// gdbprobe 用 GDBOX SDK 列出並測試 RP2040 硬體輸入裝置。
//
// 用法：
//
//	gdbprobe                 列出裝置與能力
//	gdbprobe status          OPEN 後查詢 STATUS
//	gdbprobe move X Y        只移動游標並量測落點（會移動真實游標）
//	gdbprobe click X Y       移動並左鍵點擊（會真的點下去）
//	gdbprobe key A           按一次指定按鍵
//
// 全部經由 gdb.dll（GDBOX SDK）；MDA 不再自己實作 Vendor HID 協議。
// 要看未被 SDK 接受的 HID 介面（例如別家的 0xFF60 集合），用 FirmwareManageTool
// 的 probe：cargo run --release --example probe。
package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	gdb "github.com/nousresearch/gdbox-sdk"
)

func main() {
	if len(os.Args) < 2 {
		listDevices()
		return
	}
	args := os.Args[1:]
	switch strings.ToLower(args[0]) {
	case "status":
		run(printStatus)
	case "move":
		run(moveTo)
	case "click":
		run(clickAt)
	case "key":
		run(pressKey)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", args[0])
		os.Exit(1)
	}
}

// listDevices 印出 SDK 認得的裝置；沒有裝置就不開 session。
func listDevices() {
	raw, err := gdb.ListJSON()
	if err != nil {
		fmt.Fprintf(os.Stderr, "list failed: %v\n", err)
		os.Exit(1)
	}
	if raw == "" || raw == "[]" {
		fmt.Println("no GDBOX device found (需要 03F0:0024 + vendor HID usage FF60:0061)")
		os.Exit(2)
	}
	fmt.Printf("devices: %s\n", raw)

	device, err := open()
	if err != nil {
		fmt.Fprintf(os.Stderr, "open failed: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = device.Close() }()
	if info, err := device.InfoJSON(); err == nil {
		fmt.Printf("info: %s\n", info)
	}
}

func run(action func(*gdb.Device, []string)) {
	device, err := open()
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect failed: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = device.Close() }()
	printRuntime(device)
	if info, err := device.InfoJSON(); err == nil {
		fmt.Printf("info: %s\n", info)
	}
	action(device, os.Args[2:])
}

// printRuntime 印出 SDK 版本與能力，診斷時先確認載入到哪一版 gdb.dll。
func printRuntime(device *gdb.Device) {
	major, minor := gdb.LibraryABIVersions()
	fmt.Printf("sdk: %s (C ABI %d.%d)\n", gdb.Version(), major, minor)
	if capabilities, err := gdb.Capabilities(); err == nil {
		fmt.Printf("capabilities: %s\n", capabilities)
	}
	if state, err := gdb.PreparePlatform(); err == nil {
		fmt.Printf("platform: %s\n", state.DpiAwareness)
	}
}

func open() (*gdb.Device, error) {
	return gdb.Open("", 1000, false, true)
}

func printStatus(device *gdb.Device, _ []string) {
	status, err := device.StatusJSON()
	if err != nil {
		fmt.Fprintf(os.Stderr, "status failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("status: %s\n", status)
}

func moveTo(device *gdb.Device, args []string) {
	x, y, ms := coords(args, "move")
	report, err := device.Mouse().MoveToOnReport(x, y, gdb.ScreenPrimary(), ms, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "move failed: %v\n", err)
		os.Exit(1)
	}
	printReport(report)
}

func clickAt(device *gdb.Device, args []string) {
	x, y, ms := coords(args, "click")
	report, err := device.Mouse().MoveToOnReport(x, y, gdb.ScreenPrimary(), ms, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "move failed: %v\n", err)
		os.Exit(1)
	}
	if err := device.Mouse().ClickTimed(gdb.Left, 1, 40, 1, 0); err != nil {
		fmt.Fprintf(os.Stderr, "click failed: %v\n", err)
		os.Exit(1)
	}
	printReport(report)
}

func pressKey(device *gdb.Device, args []string) {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: gdbprobe key <A-Z|0-9|space|enter|esc>")
		os.Exit(1)
	}
	page, usage, err := parseKey(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	if err := device.Keyboard().LongPress(page, usage, 40); err != nil {
		fmt.Fprintf(os.Stderr, "key failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("pressed %s\n", args[0])
}

// coords 解析 X Y [duration_ms]；duration 省略時用 SDK 的自然時長（0）。
func coords(args []string, name string) (int32, int32, uint32) {
	if len(args) < 2 || len(args) > 3 {
		fmt.Fprintf(os.Stderr, "usage: gdbprobe %s X Y [duration_ms]\n", name)
		os.Exit(1)
	}
	x, errX := strconv.Atoi(args[0])
	y, errY := strconv.Atoi(args[1])
	if errX != nil || errY != nil {
		fmt.Fprintln(os.Stderr, "coordinates must be integers")
		os.Exit(1)
	}
	var duration uint32
	if len(args) == 3 {
		value, err := strconv.Atoi(args[2])
		if err != nil || value < 0 {
			fmt.Fprintln(os.Stderr, "duration must be a non-negative integer")
			os.Exit(1)
		}
		duration = uint32(value)
	}
	return int32(x), int32(y), duration
}

func parseKey(token string) (uint16, uint16, error) {
	const keyboardPage = 7
	switch strings.ToLower(token) {
	case "space":
		return keyboardPage, 0x2C, nil
	case "enter":
		return keyboardPage, 0x28, nil
	case "esc", "escape":
		return keyboardPage, 0x29, nil
	}
	upper := strings.ToUpper(token)
	if len(upper) == 1 {
		c := upper[0]
		switch {
		case c >= 'A' && c <= 'Z':
			return keyboardPage, uint16(c-'A') + 0x04, nil
		case c >= '0' && c <= '9':
			if c == '0' {
				return keyboardPage, 0x27, nil
			}
			return keyboardPage, uint16(c-'1') + 0x1E, nil
		}
	}
	return 0, 0, fmt.Errorf("unsupported key %q", token)
}

// printReport 印出 SDK 量到的落點：requested 是管線要的位置，applied 是游標實際
// 停在的位置，clamped 表示目標落在 anchor 外被夾進矩形。
func printReport(report gdb.MoveReport) {
	fmt.Printf("landing: requested=(%d,%d) applied=(%d,%d) error_px=%d clamped=%v elapsed=%s\n",
		report.RequestedX, report.RequestedY,
		report.AppliedX, report.AppliedY,
		report.ErrorPx(), report.Clamped,
		time.Duration(report.ElapsedUs)*time.Microsecond)
}
