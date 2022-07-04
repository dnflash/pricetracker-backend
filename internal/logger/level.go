package logger

import (
	"github.com/pkg/errors"
	"strings"
)

//go:generate go run golang.org/x/tools/cmd/stringer -type=Level -linecomment

type Level int

const (
	LevelOff   Level = iota // OFF
	LevelFatal              // FATAL
	LevelError              // ERROR
	LevelWarn               // WARN
	LevelInfo               // INFO
	LevelDebug              // DEBUG
	LevelTrace              // TRACE
)

var levelMap = map[string]Level{
	"OFF":   LevelOff,
	"FATAL": LevelFatal,
	"ERROR": LevelError,
	"WARN":  LevelWarn,
	"INFO":  LevelInfo,
	"DEBUG": LevelDebug,
	"TRACE": LevelTrace,
}

func ParseLevel(s string) (Level, error) {
	level, ok := levelMap[strings.ToUpper(s)]
	if !ok {
		return -1, errors.Errorf("invalid level: %s", s)
	}
	return level, nil
}
