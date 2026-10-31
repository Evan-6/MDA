package gdbinput

import (
	maa "github.com/MaaXYZ/maa-framework-go/v4"
)

// taskSink 在任務結束時釋放按鍵並關閉 device session。
//
// 沒有這個收尾，任務中途失敗時 KeyDown 過的鍵或按住的滑鼠按鈕會留在裝置上，
// 使用者的實體鍵鼠就會被「卡住」。
type taskSink struct{}

var _ maa.TaskerEventSink = &taskSink{}

func (s *taskSink) OnTaskerTask(tasker *maa.Tasker, event maa.EventStatus, detail maa.TaskerTaskDetail) {
	switch event {
	case maa.EventStatusSucceeded, maa.EventStatusFailed:
		sessionMu.Lock()
		active := activeDevice != nil
		sessionMu.Unlock()
		if !active {
			return
		}
		logger.Info().
			Str("entry", detail.Entry).
			Int("status", int(event)).
			Msg("releasing hardware input after task")
		CloseSession()
	}
}

// EnsureTaskSink 註冊任務結束的清理回呼。
func EnsureTaskSink() {
	maa.AgentServerAddTaskerSink(&taskSink{})
}
