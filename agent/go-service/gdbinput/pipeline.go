package gdbinput

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// walkPipeline 走訪 pipeline 目錄下所有 JSON（允許 // 與 /* */ 註解），逐檔交給 fn。
func walkPipeline(root string, fn func(path string, doc map[string]any)) error {
	info, err := os.Stat(root)
	if err != nil {
		return fmt.Errorf("pipeline dir %s: %w", root, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", root)
	}
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(path), ".json") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var doc map[string]any
		if err := json.Unmarshal(stripJSONC(raw), &doc); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		fn(path, doc)
		return nil
	})
}

// stripJSONC 移除 JSON 的 // 與 /* */ 註解，字串內容保持原樣。
func stripJSONC(raw []byte) []byte {
	out := make([]byte, 0, len(raw))
	inString := false
	escaped := false
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if inString {
			out = append(out, c)
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
			continue
		}
		switch {
		case c == '"':
			inString = true
			out = append(out, c)
		case c == '/' && i+1 < len(raw) && raw[i+1] == '/':
			for i < len(raw) && raw[i] != '\n' {
				i++
			}
			if i < len(raw) {
				out = append(out, '\n')
			}
		case c == '/' && i+1 < len(raw) && raw[i+1] == '*':
			i += 2
			for i+1 < len(raw) && !(raw[i] == '*' && raw[i+1] == '/') {
				i++
			}
			i++ // 吃掉結尾的 '/'
		default:
			out = append(out, c)
		}
	}
	return out
}
