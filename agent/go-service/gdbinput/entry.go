package gdbinput

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"

	"github.com/1204244136/MDA/agent/go-service/pkg/resource"
	maa "github.com/MaaXYZ/maa-framework-go/v4"
)

var (
	overrideOnce  sync.Once
	overrideCache map[string]any
	overrideErr   error
)

// overridePayload 只從安裝目錄解析一次 pipeline，之後重用。
func overridePayload() (map[string]any, error) {
	overrideOnce.Do(func() {
		root := filepath.Join(resource.GetResourceBase(), "pipeline")
		if root == "" || root == "pipeline" {
			// resource base 尚未回報時退回標準路徑。
			if resolved := resource.FindResource("pipeline"); resolved != "" {
				root = resolved
			}
		}
		overrideCache, overrideErr = BuildOverride(root)
		if overrideErr == nil {
			logger.Info().Str("pipeline", root).Int("nodes", len(overrideCache)).
				Msg("pipeline override prepared")
		}
	})
	return overrideCache, overrideErr
}

// HandleEntry 由 context sink 在任務第一個節點啟動時呼叫。
//
// 只負責：讀 UI 選項 → 偵測裝置 → 開 SDK session → 套用輸入 override。
// 任何一步失敗都只記錄並放行，讓任務改用 MaaFramework 內建控制方式。
func HandleEntry(ctx *maa.Context, entryNode string) {
	cfg := LoadConfig()
	// UI 的「硬體輸入」選項由 global_option 寫進入口節點的 attach；沒有就是跟隨設定檔。
	mode := hardwareModeFromNode(ctx, entryNode)
	present := platformDevicePresent()
	if !cfg.EnabledWithMode(present, mode) {
		return
	}
	if !present {
		logger.Warn().Msg("enabled but no GDBOX device found, using built-in input")
		return
	}

	// 先確保 session 可用，再套用 override；順序顛倒會讓節點被改寫成硬體動作卻沒有裝置可用。
	if _, _, err := ensureSession(cfg); err != nil {
		logger.Error().Err(err).Msg("session setup failed, using built-in input")
		return
	}

	payload, err := overridePayload()
	if err != nil {
		logger.Error().Err(err).Msg("override build failed, using built-in input")
		return
	}
	if err := ctx.OverridePipeline(payload); err != nil {
		logger.Error().Err(err).Msg("OverridePipeline failed, using built-in input")
		return
	}
	logger.Info().Int("nodes", len(payload)).Str("mode", mode).Msg("hardware input override applied")
	return
}

// hardwareModeFromNode 讀任務入口節點上由 UI 選項寫入的 attach.hardware_input。
//
// 「硬體輸入」是 interface.json 的 global_option：選了 On／Off 之後，Client 會把
// `attach: {hardware_input: "on"|"off"}` 併進每個任務的入口節點；Auto 不寫，於是
// 這裡回空字串，交回設定檔與環境變數決定。
func hardwareModeFromNode(ctx *maa.Context, entryNode string) string {
	if ctx == nil || entryNode == "" {
		return ""
	}
	raw, err := ctx.GetNodeJSON(entryNode)
	if err != nil {
		logger.Debug().Err(err).Str("node", entryNode).Msg("entry node is unreadable")
		return ""
	}
	return parseHardwareMode(raw)
}

// parseHardwareMode 從節點 JSON 取 attach.hardware_input，只認 on／off。
func parseHardwareMode(raw string) string {
	var doc struct {
		Attach map[string]any `json:"attach"`
	}
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return ""
	}
	value, _ := doc.Attach["hardware_input"].(string)
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "on":
		return "on"
	case "off":
		return "off"
	}
	return ""
}
