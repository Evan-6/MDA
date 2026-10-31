module github.com/1204244136/MDA/agent/go-service

go 1.25.6

require (
	github.com/MaaXYZ/maa-framework-go/v4 v4.0.0-beta.19
	github.com/nousresearch/gdbox-sdk v0.0.0
	github.com/rs/zerolog v1.34.0
	golang.org/x/sys v0.22.0
)

// gdbox-sdk 是 FirmwareManageTool_Rust 的 GDBOX SDK（Go 綁定 + gdb.dll），
// 以 git submodule 釘在 agent/go-service/gdbox-sdk，見 tools/gdbox.py。
replace github.com/nousresearch/gdbox-sdk => ./gdbox-sdk/sdk/go

require (
	github.com/ebitengine/purego v0.9.1 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.19 // indirect
)
