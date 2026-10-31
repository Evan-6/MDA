package gdbinput

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const repoPipeline = "../../../assets/resource/pipeline"

func TestStripJSONC(t *testing.T) {
	input := `{
		// 行註解
		"a": "含 // 的字串", /* 區塊
		註解 */
		"b": 1 // 尾端註解
	}`
	var doc map[string]any
	if err := json.Unmarshal(stripJSONC([]byte(input)), &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if doc["a"] != "含 // 的字串" {
		t.Fatalf("string content was altered: %v", doc["a"])
	}
	if doc["b"].(float64) != 1 {
		t.Fatalf("b = %v", doc["b"])
	}
}

func TestResolveClickPoint(t *testing.T) {
	box := maaRect{100, 50, 40, 20} // 中心 (120,60)

	// target 缺省 → 用辨識框中心
	if x, y, err := resolveClickPoint(box, clickParam{}); err != nil || x != 120 || y != 60 {
		t.Fatalf("default target = (%d,%d) err=%v", x, y, err)
	}
	// target true → 同上
	if x, y, _ := resolveClickPoint(box, clickParam{Target: true}); x != 120 || y != 60 {
		t.Fatalf("target=true = (%d,%d)", x, y)
	}
	// target_offset 先位移再取中心
	x, y, err := resolveClickPoint(box, clickParam{Target: true, TargetOffset: []int32{-20, 20, 0, 0}})
	if err != nil || x != 100 || y != 80 {
		t.Fatalf("offset = (%d,%d) err=%v", x, y, err)
	}
	// target 明確指定矩形
	x, y, err = resolveClickPoint(box, clickParam{Target: []any{float64(0), float64(0), float64(10), float64(10)}})
	if err != nil || x != 5 || y != 5 {
		t.Fatalf("explicit target = (%d,%d) err=%v", x, y, err)
	}
	// 擴張 offset（w/h）會改變中心
	x, y, _ = resolveClickPoint(box, clickParam{Target: true, TargetOffset: []int32{0, 0, 10, 10}})
	if x != 125 || y != 65 {
		t.Fatalf("expand = (%d,%d)", x, y)
	}
	// target=false 不支援
	if _, _, err := resolveClickPoint(box, clickParam{Target: false}); err == nil {
		t.Fatal("target=false should fail")
	}
	// target 只給點 [x,y]：MaaFramework 允許，pipeline 有 8 個節點這樣寫
	if x, y, err := resolveClickPoint(box, clickParam{Target: []any{float64(150), float64(650)}}); err != nil || x != 150 || y != 650 {
		t.Fatalf("point target = (%d,%d) err=%v", x, y, err)
	}
	// 點 target 也能疊 target_offset
	if x, y, err := resolveClickPoint(box, clickParam{Target: []any{float64(150), float64(650)}, TargetOffset: []int32{-10, 5}}); err != nil || x != 140 || y != 655 {
		t.Fatalf("point target + offset = (%d,%d) err=%v", x, y, err)
	}
	// 其他長度要報錯，不能靜默取錯座標
	if _, _, err := resolveClickPoint(box, clickParam{Target: []any{float64(1), float64(2), float64(3)}}); err == nil {
		t.Fatal("3-value target should fail")
	}
}

func TestVKToKey(t *testing.T) {
	cases := map[int32]uint16{
		'A': 0x04, 'D': 0x07, 'S': 0x16, 'Z': 0x1D,
		'0': 0x27, '1': 0x1E, '9': 0x26,
		0x20: 0x2C, 0x0D: 0x28, 0x1B: 0x29,
		0x10: 0xE1, 0x11: 0xE0, 0x12: 0xE2,
	}
	for vk, usage := range cases {
		key, err := vkToKey(vk)
		if err != nil {
			t.Fatalf("vk 0x%02X: %v", vk, err)
		}
		if key.Page != 7 || key.Usage != usage {
			t.Fatalf("vk 0x%02X = page %d usage 0x%02X, want 0x%02X", vk, key.Page, key.Usage, usage)
		}
	}
	if _, err := vkToKey(0xFFFF); err == nil {
		t.Fatal("unknown vk should fail")
	}
}

func TestBuildOverrideCoversPipelineInputs(t *testing.T) {
	root := filepath.Clean(repoPipeline)
	override, err := BuildOverride(root)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(override) < 400 {
		t.Fatalf("override covers %d nodes, expected the whole input surface (≈433)", len(override))
	}

	click := override["AccountNurturingCharacterBreakthroughClickRedDot"]
	if click == nil {
		t.Fatal("expected Click node in override")
	}
	action := click.(map[string]any)["action"].(map[string]any)
	param := action["param"].(map[string]any)
	if param["custom_action"] != "GdbClick" {
		t.Fatalf("custom_action = %v", param["custom_action"])
	}
	raw, err := json.Marshal(param["custom_action_param"])
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(raw) != `{"target_offset":[-20,20,0,0]}` {
		t.Fatalf("param passthrough = %s", raw)
	}

	swipe := override["CubeEnhancementSwipeUp"]
	if swipe == nil {
		t.Fatal("expected Swipe node in override")
	}
	swipeParam := swipe.(map[string]any)["action"].(map[string]any)["param"].(map[string]any)
	if swipeParam["custom_action"] != "GdbSwipe" {
		t.Fatalf("swipe custom_action = %v", swipeParam["custom_action"])
	}

	key := override["FastBurstClickKeyA"]
	if key == nil {
		t.Fatal("expected ClickKey node in override")
	}
	keyParam := key.(map[string]any)["action"].(map[string]any)["param"].(map[string]any)
	if keyParam["custom_action"] != "GdbClickKey" {
		t.Fatalf("key custom_action = %v", keyParam["custom_action"])
	}
	raw, _ = json.Marshal(keyParam["custom_action_param"])
	if string(raw) != `{"key":65}` {
		t.Fatalf("key param = %s", raw)
	}
}

// TestBuildOverrideCoversScrollAndLongPress 用合成 pipeline 驗三種新動作被改寫。
//
// repo 的 pipeline 目前沒有任何 Scroll／LongPress／LongPressKey 節點，所以只能用
// 臨時目錄造一份，否則這條路徑在測試裡永遠不會被走到。
func TestBuildOverrideCoversScrollAndLongPress(t *testing.T) {
	dir := t.TempDir()
	doc := `{
		"ScrollShopList": {"action": {"type": "Scroll", "param": {"target": true, "dy": -360}}},
		"LongPressItem": {"action": {"type": "LongPress", "param": {"target": [10, 20, 30, 40], "duration": 1500, "contact": 1}}},
		"LongPressKeyW": {"action": {"type": "LongPressKey", "param": {"key": 87, "duration": 800}}}
	}`
	if err := os.WriteFile(filepath.Join(dir, "synthetic.json"), []byte(doc), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	override, err := BuildOverride(dir)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	want := map[string]string{
		"ScrollShopList": "GdbScroll",
		"LongPressItem":  "GdbLongPress",
		"LongPressKeyW":  "GdbLongPressKey",
	}
	for node, custom := range want {
		entry, ok := override[node]
		if !ok {
			t.Fatalf("%s missing from override", node)
		}
		param := entry.(map[string]any)["action"].(map[string]any)["param"].(map[string]any)
		if param["custom_action"] != custom {
			t.Fatalf("%s custom_action = %v, want %s", node, param["custom_action"], custom)
		}
		if _, ok := param["custom_action_param"]; !ok {
			t.Fatalf("%s lost its param", node)
		}
	}
}

func TestBuildOverrideLeavesNonInputNodesAlone(t *testing.T) {
	root := filepath.Clean(repoPipeline)
	override, err := BuildOverride(root)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if _, ok := override["QuotaDisplayMain"]; ok {
		t.Fatal("Custom action node must not be overridden")
	}
	if _, ok := override["EquipmentRerollMain"]; ok {
		t.Fatal("TaskEntry node must not be overridden")
	}
}

func TestConfigEnablement(t *testing.T) {
	t.Setenv("MDA_GDB_INPUT", "")
	cfg := DefaultConfig()
	if cfg.Enabled != "auto" {
		t.Fatalf("default enabled = %q", cfg.Enabled)
	}
	if !cfg.EnabledWith(true) {
		t.Fatal("auto with device should enable")
	}
	if cfg.EnabledWith(false) {
		t.Fatal("auto without device should not enable")
	}
	cfg.Enabled = "on"
	if !cfg.EnabledWith(false) {
		t.Fatal("on should force enable")
	}
	cfg.Enabled = "off"
	if cfg.EnabledWith(true) {
		t.Fatal("off should force disable")
	}

	t.Setenv("MDA_GDB_INPUT", "1")
	if !cfg.EnabledWith(false) {
		t.Fatal("env override should enable")
	}
	t.Setenv("MDA_GDB_INPUT", "0")
	cfg.Enabled = "on"
	if cfg.EnabledWith(true) {
		t.Fatal("env override should disable")
	}
}

// TestAllPipelineClickTargetsResolve 掃過真實 pipeline，確認每個 Click 的 target 形狀都解析得出來。
// 實際踩過：目標寫成 [x,y] 這種點形式時舊版只接受 [x,y,w,h]，執行到該節點就報錯讓任務停下。
func TestAllPipelineClickTargetsResolve(t *testing.T) {
	root := filepath.Clean(repoPipeline)
	box := maaRect{0, 0, 10, 10}
	checked := 0
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".json") {
			return err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var doc map[string]struct {
			Action json.RawMessage `json:"action"`
		}
		if err := json.Unmarshal(stripJSONC(raw), &doc); err != nil {
			return err
		}
		for name, node := range doc {
			var act struct {
				Type  string     `json:"type"`
				Param clickParam `json:"param"`
			}
			if len(node.Action) == 0 {
				continue
			}
			if err := json.Unmarshal(node.Action, &act); err != nil {
				continue
			}
			if act.Type != "Click" {
				continue
			}
			checked++
			if _, _, err := resolveClickPoint(box, act.Param); err != nil {
				t.Errorf("%s (%s): %v", name, path, err)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk pipeline: %v", err)
	}
	if checked < 400 {
		t.Fatalf("only %d Click nodes checked, pipeline scan looks broken", checked)
	}
	t.Logf("checked %d Click nodes", checked)
}
