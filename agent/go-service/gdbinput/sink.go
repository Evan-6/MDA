package gdbinput

import (
	"sync"

	maa "github.com/MaaXYZ/maa-framework-go/v4"
)

// contextSink 在每個任務的第一個 pipeline 節點啟動時套用硬體輸入 override。
//
// 為什麼不用 pipeline 節點動作：任務入口節點原本就有 RuntimeQuotaCheck，但只有 18 個任務有，
// 而且為了覆蓋剩下的任務就得改 pipeline（新增節點或補動作）。context sink 會拿到本次任務的
// *Context，第一個節點啟動時套用一次即可，pipeline 完全不用動。
type contextSink struct {
	mu      sync.Mutex
	applied map[uint64]bool
}

// maxTrackedTasks 限制記錄的 task id 數量，避免長時間執行時無界成長。
const maxTrackedTasks = 256

func newContextSink() *contextSink {
	return &contextSink{applied: make(map[uint64]bool)}
}

// markApplied 回報這個 task 是否為第一次套用；重複的節點事件回傳 false。
func (s *contextSink) markApplied(taskID uint64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.applied[taskID] {
		return false
	}
	if len(s.applied) >= maxTrackedTasks {
		s.applied = make(map[uint64]bool)
	}
	s.applied[taskID] = true
	return true
}

func (s *contextSink) OnNodePipelineNode(ctx *maa.Context, event maa.EventStatus, detail maa.NodePipelineNodeDetail) {
	if event != maa.EventStatusStarting {
		return
	}
	// 第一個節點一定是任務入口，動作不是輸入動作，所以在這裡套用不會漏掉任何點擊。
	if !s.markApplied(detail.TaskID) {
		return
	}
	// detail.Name 是任務的第一個節點；UI 的硬體輸入選項就 attach 在該節點上。
	HandleEntry(ctx, detail.Name)
}

func (s *contextSink) OnNodeRecognitionNode(*maa.Context, maa.EventStatus, maa.NodeRecognitionNodeDetail) {
}

func (s *contextSink) OnNodeActionNode(*maa.Context, maa.EventStatus, maa.NodeActionNodeDetail) {}

func (s *contextSink) OnNodeNextList(*maa.Context, maa.EventStatus, maa.NodeNextListDetail) {}

func (s *contextSink) OnNodeRecognition(*maa.Context, maa.EventStatus, maa.NodeRecognitionDetail) {
}

func (s *contextSink) OnNodeAction(*maa.Context, maa.EventStatus, maa.NodeActionDetail) {}
