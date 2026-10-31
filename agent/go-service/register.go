package main

import (
	"github.com/1204244136/MDA/agent/go-service/common/centerpriority"
	"github.com/1204244136/MDA/agent/go-service/customburst"
	"github.com/1204244136/MDA/agent/go-service/equipmentreroll"
	"github.com/1204244136/MDA/agent/go-service/gdbinput"
	"github.com/1204244136/MDA/agent/go-service/pkg/resource"
	"github.com/1204244136/MDA/agent/go-service/taskersink/aspectratio"
	"github.com/1204244136/MDA/agent/go-service/taskersink/hdrcheck"
	"github.com/1204244136/MDA/agent/go-service/taskersink/membership"
	"github.com/1204244136/MDA/agent/go-service/taskersink/processcheck"
	"github.com/rs/zerolog/log"
)

func registerAll() {
	// Resource Sink
	resource.EnsureResourcePathSink()

	// Common Custom
	centerpriority.Register()
	equipmentreroll.Register()
	customburst.Register()

	// Pre-Check Custom
	aspectratio.Register()
	hdrcheck.Register()
	processcheck.Register()
	// 任務入口（授權驗證已移除；入口現在只負責偵測 GDBOX 硬體輸入並套用 override）
	gdbinput.Register()
	// 會員額度已停用，僅保留舊節點的相容別名
	membership.Register()

	log.Info().
		Msg("All custom components and sinks registered successfully")
}
