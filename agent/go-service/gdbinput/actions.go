package gdbinput

import (
	"sync"
	"time"

	"github.com/1204244136/MDA/agent/go-service/pkg/maafocus"
	maa "github.com/MaaXYZ/maa-framework-go/v4"
)

const hardwareUnavailable = "🍊 硬體輸入不可用，任務已停止。請確認 GDBOX 裝置已連接或關閉硬體輸入模式。"

var (
	configMu  sync.Mutex
	cachedCfg Config
	cfgLoaded time.Time
	configTTL = 5 * time.Second
)

// currentConfig 快取設定檔，避免每次點擊都讀檔。
func currentConfig() Config {
	configMu.Lock()
	defer configMu.Unlock()
	if time.Since(cfgLoaded) > configTTL {
		cachedCfg = LoadConfig()
		cfgLoaded = time.Now()
	}
	return cachedCfg
}

// holdDuration 是按鈕／按鍵按住的時間。
func holdDuration(cfg Config) time.Duration {
	return time.Duration(cfg.ClickHoldMS) * time.Millisecond
}

// GdbClickAction 取代 pipeline 的 Click 動作。
type GdbClickAction struct{}

var _ maa.CustomActionRunner = &GdbClickAction{}

func (a *GdbClickAction) Run(ctx *maa.Context, arg *maa.CustomActionArg) bool {
	cfg := currentConfig()
	var param clickParam
	if err := marshalParam(arg.CustomActionParam, &param); err != nil {
		logger.Error().Err(err).Str("node", arg.CurrentTaskName).Msg("GdbClick param")
		return false
	}
	box := maaRect{int32(arg.Box[0]), int32(arg.Box[1]), int32(arg.Box[2]), int32(arg.Box[3])}
	imgX, imgY, err := resolveClickPoint(box, param)
	if err != nil {
		logger.Error().Err(err).Str("node", arg.CurrentTaskName).Msg("GdbClick target")
		return false
	}
	client, win, err := currentSession()
	if err != nil {
		maafocus.Print(ctx, hardwareUnavailable)
		return false
	}
	screenX, screenY := win.ToScreen(imgX, imgY, cfg.ImageWidth, cfg.ImageHeight)
	if err := client.Click(win.Screen(), screenX, screenY, holdDuration(cfg)); err != nil {
		logger.Error().Err(err).Str("node", arg.CurrentTaskName).
			Int32("screen_x", screenX).Int32("screen_y", screenY).
			Msg("GdbClick failed")
		return false
	}
	logger.Debug().
		Str("node", arg.CurrentTaskName).
		Int32("image_x", imgX).Int32("image_y", imgY).
		Int32("screen_x", screenX).Int32("screen_y", screenY).
		Msg("click sent")
	return true
}

// GdbSwipeAction 取代 pipeline 的 Swipe 動作。
type GdbSwipeAction struct{}

var _ maa.CustomActionRunner = &GdbSwipeAction{}

func (a *GdbSwipeAction) Run(ctx *maa.Context, arg *maa.CustomActionArg) bool {
	cfg := currentConfig()
	var param swipeParam
	if err := marshalParam(arg.CustomActionParam, &param); err != nil {
		logger.Error().Err(err).Str("node", arg.CurrentTaskName).Msg("GdbSwipe param")
		return false
	}
	if len(param.Begin) != 2 || len(param.End) != 2 {
		logger.Error().Str("node", arg.CurrentTaskName).Msg("GdbSwipe needs begin/end")
		return false
	}
	client, win, err := currentSession()
	if err != nil {
		maafocus.Print(ctx, hardwareUnavailable)
		return false
	}
	x1, y1 := win.ToScreen(param.Begin[0], param.Begin[1], cfg.ImageWidth, cfg.ImageHeight)
	x2, y2 := win.ToScreen(param.End[0], param.End[1], cfg.ImageWidth, cfg.ImageHeight)
	duration := time.Duration(param.Duration) * time.Millisecond
	if err := client.Drag(win.Screen(), x1, y1, x2, y2, duration, time.Duration(param.EndHold)*time.Millisecond); err != nil {
		logger.Error().Err(err).Str("node", arg.CurrentTaskName).Msg("GdbSwipe failed")
		return false
	}
	logger.Debug().Str("node", arg.CurrentTaskName).
		Int32("x1", x1).Int32("y1", y1).Int32("x2", x2).Int32("y2", y2).
		Msg("swipe sent")
	return true
}

// GdbClickKeyAction 取代 pipeline 的 ClickKey 動作（按下後放開指定鍵）。
type GdbClickKeyAction struct{}

var _ maa.CustomActionRunner = &GdbClickKeyAction{}

func (a *GdbClickKeyAction) Run(ctx *maa.Context, arg *maa.CustomActionArg) bool {
	var param keyParam
	if err := marshalParam(arg.CustomActionParam, &param); err != nil {
		logger.Error().Err(err).Str("node", arg.CurrentTaskName).Msg("GdbClickKey param")
		return false
	}
	key, err := vkToKey(param.Key)
	if err != nil {
		logger.Error().Err(err).Str("node", arg.CurrentTaskName).Msg("GdbClickKey")
		return false
	}
	client, _, err := currentSession()
	if err != nil {
		maafocus.Print(ctx, hardwareUnavailable)
		return false
	}
	hold := holdDuration(currentConfig())
	if err := client.TapKey(key.Page, key.Usage, hold); err != nil {
		logger.Error().Err(err).Str("node", arg.CurrentTaskName).Msg("GdbClickKey failed")
		return false
	}
	return true
}

// GdbScrollAction 取代 pipeline 的 Scroll 動作。
type GdbScrollAction struct{}

var _ maa.CustomActionRunner = &GdbScrollAction{}

func (a *GdbScrollAction) Run(ctx *maa.Context, arg *maa.CustomActionArg) bool {
	cfg := currentConfig()
	var param scrollParam
	if err := marshalParam(arg.CustomActionParam, &param); err != nil {
		logger.Error().Err(err).Str("node", arg.CurrentTaskName).Msg("GdbScroll param")
		return false
	}
	if param.DX == 0 && param.DY == 0 {
		logger.Debug().Str("node", arg.CurrentTaskName).Msg("GdbScroll with dx=dy=0, nothing to do")
		return true
	}
	box := maaRect{int32(arg.Box[0]), int32(arg.Box[1]), int32(arg.Box[2]), int32(arg.Box[3])}
	imgX, imgY, err := resolveClickPoint(box, clickParam{Target: param.Target, TargetOffset: param.TargetOffset})
	if err != nil {
		logger.Error().Err(err).Str("node", arg.CurrentTaskName).Msg("GdbScroll target")
		return false
	}
	client, win, err := currentSession()
	if err != nil {
		maafocus.Print(ctx, hardwareUnavailable)
		return false
	}
	screenX, screenY := win.ToScreen(imgX, imgY, cfg.ImageWidth, cfg.ImageHeight)
	if err := client.Wheel(win.Screen(), screenX, screenY, param.DY, param.DX); err != nil {
		logger.Error().Err(err).Str("node", arg.CurrentTaskName).
			Int32("dx", param.DX).Int32("dy", param.DY).
			Int32("screen_x", screenX).Int32("screen_y", screenY).
			Msg("GdbScroll failed")
		return false
	}
	logger.Debug().Str("node", arg.CurrentTaskName).
		Int32("dx", param.DX).Int32("dy", param.DY).
		Int32("screen_x", screenX).Int32("screen_y", screenY).
		Msg("scroll sent")
	return true
}

// GdbLongPressAction 取代 pipeline 的 LongPress 動作。
type GdbLongPressAction struct{}

var _ maa.CustomActionRunner = &GdbLongPressAction{}

func (a *GdbLongPressAction) Run(ctx *maa.Context, arg *maa.CustomActionArg) bool {
	cfg := currentConfig()
	var param longPressParam
	if err := marshalParam(arg.CustomActionParam, &param); err != nil {
		logger.Error().Err(err).Str("node", arg.CurrentTaskName).Msg("GdbLongPress param")
		return false
	}
	box := maaRect{int32(arg.Box[0]), int32(arg.Box[1]), int32(arg.Box[2]), int32(arg.Box[3])}
	imgX, imgY, err := resolveClickPoint(box, clickParam{Target: param.Target, TargetOffset: param.TargetOffset})
	if err != nil {
		logger.Error().Err(err).Str("node", arg.CurrentTaskName).Msg("GdbLongPress target")
		return false
	}
	client, win, err := currentSession()
	if err != nil {
		maafocus.Print(ctx, hardwareUnavailable)
		return false
	}
	screenX, screenY := win.ToScreen(imgX, imgY, cfg.ImageWidth, cfg.ImageHeight)
	hold := time.Duration(param.Duration) * time.Millisecond
	if hold <= 0 {
		// MaaFramework 的 LongPress 預設 1000 ms；自訂動作拿到的是原始 param。
		hold = 1000 * time.Millisecond
	}
	if err := client.Press(win.Screen(), uint8(param.Contact), screenX, screenY, hold); err != nil {
		logger.Error().Err(err).Str("node", arg.CurrentTaskName).
			Int32("contact", param.Contact).
			Int32("screen_x", screenX).Int32("screen_y", screenY).
			Msg("GdbLongPress failed")
		return false
	}
	logger.Debug().Str("node", arg.CurrentTaskName).
		Int32("contact", param.Contact).Dur("hold", hold).
		Int32("screen_x", screenX).Int32("screen_y", screenY).
		Msg("long press sent")
	return true
}

// GdbLongPressKeyAction 取代 pipeline 的 LongPressKey 動作。
type GdbLongPressKeyAction struct{}

var _ maa.CustomActionRunner = &GdbLongPressKeyAction{}

func (a *GdbLongPressKeyAction) Run(ctx *maa.Context, arg *maa.CustomActionArg) bool {
	var param longPressKeyParam
	if err := marshalParam(arg.CustomActionParam, &param); err != nil {
		logger.Error().Err(err).Str("node", arg.CurrentTaskName).Msg("GdbLongPressKey param")
		return false
	}
	key, err := vkToKey(param.Key)
	if err != nil {
		logger.Error().Err(err).Str("node", arg.CurrentTaskName).Msg("GdbLongPressKey")
		return false
	}
	client, _, err := currentSession()
	if err != nil {
		maafocus.Print(ctx, hardwareUnavailable)
		return false
	}
	hold := time.Duration(param.Duration) * time.Millisecond
	if hold <= 0 {
		hold = 1000 * time.Millisecond
	}
	if err := client.TapKey(key.Page, key.Usage, hold); err != nil {
		logger.Error().Err(err).Str("node", arg.CurrentTaskName).Msg("GdbLongPressKey failed")
		return false
	}
	return true
}

// GdbKeyDownAction 取代 pipeline 的 KeyDown 動作。
type GdbKeyDownAction struct{}

var _ maa.CustomActionRunner = &GdbKeyDownAction{}

func (a *GdbKeyDownAction) Run(ctx *maa.Context, arg *maa.CustomActionArg) bool {
	return runKey(ctx, arg, true)
}

// GdbKeyUpAction 取代 pipeline 的 KeyUp 動作。
type GdbKeyUpAction struct{}

var _ maa.CustomActionRunner = &GdbKeyUpAction{}

func (a *GdbKeyUpAction) Run(ctx *maa.Context, arg *maa.CustomActionArg) bool {
	return runKey(ctx, arg, false)
}

func runKey(ctx *maa.Context, arg *maa.CustomActionArg, down bool) bool {
	var param keyParam
	if err := marshalParam(arg.CustomActionParam, &param); err != nil {
		logger.Error().Err(err).Str("node", arg.CurrentTaskName).Msg("key param")
		return false
	}
	key, err := vkToKey(param.Key)
	if err != nil {
		logger.Error().Err(err).Str("node", arg.CurrentTaskName).Msg("key")
		return false
	}
	client, _, err := currentSession()
	if err != nil {
		maafocus.Print(ctx, hardwareUnavailable)
		return false
	}
	if err := client.Key(key.Page, key.Usage, down); err != nil {
		logger.Error().Err(err).Str("node", arg.CurrentTaskName).Bool("down", down).Msg("key failed")
		return false
	}
	return true
}
