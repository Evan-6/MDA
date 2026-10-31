package gdbinput

import (
	"encoding/json"
	"fmt"
)

// inputActionMap 把 pipeline 內建的輸入動作映射到 Go 端的硬體動作名稱。
var inputActionMap = map[string]string{
	"Click":        "GdbClick",
	"Swipe":        "GdbSwipe",
	"ClickKey":     "GdbClickKey",
	"KeyDown":      "GdbKeyDown",
	"KeyUp":        "GdbKeyUp",
	"Scroll":       "GdbScroll",
	"LongPress":    "GdbLongPress",
	"LongPressKey": "GdbLongPressKey",
}

// BuildOverride 掃描 pipeline 目錄，把所有內建輸入動作改寫成 GDBOX 自訂動作。
//
// 只替換 action 區塊，辨識、next、pre_delay/post_delay 等欄位由 MaaFramework 合併保留；
// 原動作的 param（target、target_offset、begin、end、duration、key…）原封不動傳給
// 自訂動作，因此行為語義仍寫在 pipeline 裡。
func BuildOverride(pipelineRoot string) (map[string]any, error) {
	out := make(map[string]any)
	err := walkPipeline(pipelineRoot, func(path string, doc map[string]any) {
		for name, rawNode := range doc {
			node, ok := rawNode.(map[string]any)
			if !ok {
				continue
			}
			action, ok := node["action"].(map[string]any)
			if !ok {
				continue
			}
			actionType, _ := action["type"].(string)
			customName, ok := inputActionMap[actionType]
			if !ok {
				continue
			}
			param := action["param"]
			customAction := map[string]any{
				"type": "Custom",
				"param": map[string]any{
					"custom_action": customName,
				},
			}
			if param != nil {
				customAction["param"].(map[string]any)["custom_action_param"] = param
			}
			out[name] = map[string]any{"action": customAction}
		}
	})
	if err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no input actions found under %s", pipelineRoot)
	}
	return out, nil
}

// clickParam 是 Click 動作的 pipeline param。
type clickParam struct {
	Target       any     `json:"target"`
	TargetOffset []int32 `json:"target_offset"`
}

// swipeParam 是 Swipe 動作的 pipeline param。
type swipeParam struct {
	Begin    []int32 `json:"begin"`
	End      []int32 `json:"end"`
	Duration int32   `json:"duration"`
	EndHold  int32   `json:"end_hold"`
}

// keyParam 是 ClickKey / KeyDown / KeyUp 動作的 pipeline param。
type keyParam struct {
	Key int32 `json:"key"`
}

// scrollParam 是 Scroll 動作的 pipeline param。
//
// dx／dy 的單位是 WHEEL_DELTA（120 = 一格），正值分別向右／向上滾。
type scrollParam struct {
	Target       any     `json:"target"`
	TargetOffset []int32 `json:"target_offset"`
	DX           int32   `json:"dx"`
	DY           int32   `json:"dy"`
}

// longPressParam 是 LongPress 動作的 pipeline param。
//
// contact 是觸點編號；Win32 控制器把它當滑鼠鍵號（0 左、1 右、2 中）。
type longPressParam struct {
	Target       any     `json:"target"`
	TargetOffset []int32 `json:"target_offset"`
	Duration     int32   `json:"duration"`
	Contact      int32   `json:"contact"`
}

// longPressKeyParam 是 LongPressKey 動作的 pipeline param。
type longPressKeyParam struct {
	Key      int32 `json:"key"`
	Duration int32 `json:"duration"`
}

// resolveClickPoint 依 MaaFramework 的 Click 語義計算點擊點（影像座標）。
//
// box 是本次辨識命中的矩形；target 為 true 或缺省時使用 box，為 [x,y,w,h] 時使用該矩形；
// target_offset 為 [dx,dy,dw,dh]，先擴張矩形再加位移，最後取中心點。
func resolveClickPoint(box maaRect, param clickParam) (int32, int32, error) {
	rect := [4]int32{box[0], box[1], box[2], box[3]}
	switch target := param.Target.(type) {
	case nil:
	case bool:
		if !target {
			return 0, 0, fmt.Errorf("Click with target=false is not supported by the GDBOX action")
		}
	case []any:
		values, err := toInt32Slice(target)
		if err != nil {
			return 0, 0, fmt.Errorf("Click target must be [x,y] or [x,y,w,h]")
		}
		switch len(values) {
		case 2:
			// MaaFramework 允許只給點：[x,y] 即點擊該點，這裡用零寬高的矩形讓後續取中心。
			rect = [4]int32{values[0], values[1], 0, 0}
		case 4:
			rect = [4]int32{values[0], values[1], values[2], values[3]}
		default:
			return 0, 0, fmt.Errorf("Click target must be [x,y] or [x,y,w,h], got %d values", len(values))
		}
	default:
		return 0, 0, fmt.Errorf("unsupported Click target %T", param.Target)
	}

	offset := param.TargetOffset
	var dx, dy, dw, dh int32
	if len(offset) >= 4 {
		dx, dy, dw, dh = offset[0], offset[1], offset[2], offset[3]
	} else if len(offset) >= 2 {
		dx, dy = offset[0], offset[1]
	}

	x := rect[0] + dx
	y := rect[1] + dy
	w := rect[2] + dw
	h := rect[3] + dh
	return x + w/2, y + h/2, nil
}

func toInt32Slice(values []any) ([]int32, error) {
	out := make([]int32, 0, len(values))
	for _, v := range values {
		f, ok := v.(float64)
		if !ok {
			return nil, fmt.Errorf("not a number: %v", v)
		}
		out = append(out, int32(f))
	}
	return out, nil
}

// maaRect 是本套件內部使用的矩形型別，避免直接依賴 framework 型別。
type maaRect [4]int32

// hidKey 是 HID page + usage（SDK 鍵盤 API 的座標）。
type hidKey struct {
	Page  uint16
	Usage uint16
}

// vkToKey 把 Windows 虛擬鍵碼換成 HID page + usage。
func vkToKey(vk int32) (hidKey, error) {
	switch {
	case vk >= 'A' && vk <= 'Z':
		return hidKey{Page: 7, Usage: uint16(vk-'A') + 0x04}, nil
	case vk >= '0' && vk <= '9':
		if vk == '0' {
			return hidKey{Page: 7, Usage: 0x27}, nil
		}
		return hidKey{Page: 7, Usage: uint16(vk-'1') + 0x1E}, nil
	}
	named := map[int32]uint16{
		0x08: 0x2A, // Backspace
		0x09: 0x2B, // Tab
		0x0D: 0x28, // Enter
		0x1B: 0x29, // Escape
		0x20: 0x2C, // Space
		0x10: 0xE1, // Shift
		0x11: 0xE0, // Control
		0x12: 0xE2, // Alt
		0x25: 0x50, // Left
		0x26: 0x52, // Up
		0x27: 0x4F, // Right
		0x28: 0x51, // Down
		0x24: 0x4A, // Home
		0x23: 0x4D, // End
		0x21: 0x4B, // PageUp
		0x22: 0x4E, // PageDown
	}
	if usage, ok := named[vk]; ok {
		return hidKey{Page: 7, Usage: usage}, nil
	}
	if vk >= 0x70 && vk <= 0x7B { // F1..F12
		return hidKey{Page: 7, Usage: uint16(vk-0x70) + 0x3A}, nil
	}
	return hidKey{}, fmt.Errorf("unsupported virtual key 0x%02X", vk)
}

// marshalParam 解析自訂動作收到的 JSON 參數。
func marshalParam[T any](raw string, out *T) error {
	if raw == "" || raw == "{}" || raw == "null" {
		return nil
	}
	if err := json.Unmarshal([]byte(raw), out); err != nil {
		return fmt.Errorf("invalid custom_action_param: %w", err)
	}
	return nil
}
