package gdbinput

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

const repoInterface = "../../../assets/interface.json"

// TestParseHardwareMode 釘住 UI 選項寫進 attach 的值怎麼讀。
func TestParseHardwareMode(t *testing.T) {
	cases := []struct{ raw, want string }{
		{`{"attach":{"hardware_input":"on"}}`, "on"},
		{`{"attach":{"hardware_input":" Off "}}`, "off"},
		{`{"attach":{"hardware_input":"auto"}}`, ""},
		{`{"attach":{}}`, ""},
		{`{"action":{"type":"Click"}}`, ""},
		{`not json`, ""},
	}
	for _, c := range cases {
		if got := parseHardwareMode(c.raw); got != c.want {
			t.Fatalf("parseHardwareMode(%s) = %q, want %q", c.raw, got, c.want)
		}
	}
}

// TestEnabledPrecedence 環境變數 > UI 選項 > 設定檔 > auto。
func TestEnabledPrecedence(t *testing.T) {
	t.Setenv("MDA_GDB_INPUT", "")
	cases := []struct {
		config        string
		mode          string
		devicePresent bool
		want          bool
	}{
		{"auto", "", true, true},
		{"auto", "", false, false},
		{"auto", "on", false, true},
		{"auto", "off", true, false},
		{"on", "off", true, false},
		{"off", "on", false, true},
	}
	for _, c := range cases {
		cfg := Config{Enabled: c.config}
		if got := cfg.EnabledWithMode(c.devicePresent, c.mode); got != c.want {
			t.Fatalf("config=%s mode=%q present=%v -> %v, want %v",
				c.config, c.mode, c.devicePresent, got, c.want)
		}
	}

	// 環境變數壓過 UI 選項。
	t.Setenv("MDA_GDB_INPUT", "0")
	if got := (Config{Enabled: "on"}).EnabledWithMode(true, "on"); got {
		t.Fatal("MDA_GDB_INPUT=0 must win over the UI option")
	}
	t.Setenv("MDA_GDB_INPUT", "1")
	if got := (Config{Enabled: "off"}).EnabledWithMode(false, "off"); !got {
		t.Fatal("MDA_GDB_INPUT=1 must win over the UI option")
	}
}

// TestHardwareInputOptionCoversEveryTask 守住 UI 選項與任務入口的對齊。
//
// 「硬體輸入」是 global_option：Client 把它套到每個任務的 pipeline override，
// 而 agent 只在任務入口節點讀 attach。少一個入口就代表那個任務的開關失效，
// 而且不會有任何錯誤訊息——所以這條對齊必須有測試。
func TestHardwareInputOptionCoversEveryTask(t *testing.T) {
	raw, err := os.ReadFile(repoInterface)
	if err != nil {
		t.Skipf("interface.json 不在預期位置: %v", err)
	}
	var doc struct {
		Import       []string `json:"import"`
		GlobalOption []string `json:"global_option"`
		Setting      []struct {
			Name   string   `json:"name"`
			Option []string `json:"option"`
		} `json:"setting"`
		Option map[string]struct {
			DefaultCase string `json:"default_case"`
			Cases       []struct {
				Name             string                    `json:"name"`
				PipelineOverride map[string]map[string]any `json:"pipeline_override"`
			} `json:"cases"`
		} `json:"option"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse interface.json: %v", err)
	}

	if len(doc.GlobalOption) != 1 || doc.GlobalOption[0] != "HardwareInput" {
		t.Fatalf("global_option = %v, want [HardwareInput]", doc.GlobalOption)
	}
	if len(doc.Setting) != 1 || len(doc.Setting[0].Option) != 1 || doc.Setting[0].Option[0] != "HardwareInput" {
		t.Fatalf("setting = %+v, want one section holding HardwareInput", doc.Setting)
	}

	option, ok := doc.Option["HardwareInput"]
	if !ok {
		t.Fatal("option.HardwareInput is missing")
	}
	if option.DefaultCase != "Auto" {
		t.Fatalf("default_case = %q, want Auto", option.DefaultCase)
	}

	// 入口節點集合以 import 的任務宣告為準。
	entries := map[string]bool{}
	for _, rel := range doc.Import {
		path := filepath.Join(filepath.Dir(repoInterface), rel)
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		var taskDoc struct {
			Task []struct {
				Entry string `json:"entry"`
			} `json:"task"`
		}
		if err := json.Unmarshal(body, &taskDoc); err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, task := range taskDoc.Task {
			entries[task.Entry] = true
		}
	}
	if len(entries) < 20 {
		t.Fatalf("only %d task entries found, the import list looks wrong", len(entries))
	}

	want := map[string]string{"On": "on", "Off": "off"}
	for _, c := range option.Cases {
		value, needsOverride := want[c.Name]
		if !needsOverride {
			if len(c.PipelineOverride) != 0 {
				t.Fatalf("case %s must not override anything, got %d nodes", c.Name, len(c.PipelineOverride))
			}
			continue
		}
		if len(c.PipelineOverride) != len(entries) {
			t.Fatalf("case %s overrides %d nodes, want %d (one per task entry)",
				c.Name, len(c.PipelineOverride), len(entries))
		}
		for entry := range entries {
			node, ok := c.PipelineOverride[entry]
			if !ok {
				t.Fatalf("case %s does not cover task entry %s", c.Name, entry)
			}
			attach, _ := node["attach"].(map[string]any)
			if attach["hardware_input"] != value {
				t.Fatalf("case %s / %s attach = %v, want hardware_input=%s", c.Name, entry, attach, value)
			}
		}
	}
}
