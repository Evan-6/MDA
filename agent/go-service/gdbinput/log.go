package gdbinput

import "github.com/rs/zerolog/log"

// logger 是本套件的日誌入口，固定帶上 component 欄位。
//
// 訊息本身不再重複模組名，套件改名時只要動這一行；其餘呼叫端一律用 logger
// 而不是全域 log，避免模組身份又散回每一條訊息裡。
var logger = log.With().Str("component", "gdbinput").Logger()
