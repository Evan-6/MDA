package gdbinput

import (
	maa "github.com/MaaXYZ/maa-framework-go/v4"
)

// Register 註冊硬體輸入動作。
//
// 硬體輸入的套用點是 context sink（見 sink.go）：每個任務第一個節點啟動時呼叫 HandleEntry，
// 再把 pipeline 內建輸入動作改寫成下面這些動作（見 BuildOverride）。pipeline 完全不用改。
// 動作本身只是薄包裝，協議由 GDBOX SDK（agent/go-service/gdbox-sdk）負責；
// 未啟用硬體模式時只記錄一行，任務走原本的內建控制方式。
func Register() {
	maa.AgentServerRegisterCustomAction("GdbClick", &GdbClickAction{})
	maa.AgentServerRegisterCustomAction("GdbSwipe", &GdbSwipeAction{})
	maa.AgentServerRegisterCustomAction("GdbClickKey", &GdbClickKeyAction{})
	maa.AgentServerRegisterCustomAction("GdbKeyDown", &GdbKeyDownAction{})
	maa.AgentServerRegisterCustomAction("GdbKeyUp", &GdbKeyUpAction{})
	maa.AgentServerRegisterCustomAction("GdbScroll", &GdbScrollAction{})
	maa.AgentServerRegisterCustomAction("GdbLongPress", &GdbLongPressAction{})
	maa.AgentServerRegisterCustomAction("GdbLongPressKey", &GdbLongPressKeyAction{})
	// 先把 override 解析好（同步一次即可，約 20 ms），避免第一個任務的入口回呼臨時解析 JSON。
	if _, err := overridePayload(); err != nil {
		logger.Error().Err(err).Msg("override prebuild failed")
	}
	// 每個任務開始時套用 override（不需要 pipeline 上有任何入口動作）。
	maa.AgentServerAddContextSink(newContextSink())
	// 任務結束時釋放按鍵並關閉 session，避免留下按住的鍵。
	EnsureTaskSink()
}
