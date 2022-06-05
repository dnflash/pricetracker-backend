package logger

import (
	"fmt"
	"io"
	"log"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type logger struct {
	logger *log.Logger
	level  Level
}

func (l *logger) ErrorEnabled() bool {
	return l.level >= LevelError
}
func (l *logger) WarnEnabled() bool {
	return l.level >= LevelWarn
}
func (l *logger) InfoEnabled() bool {
	return l.level >= LevelInfo
}
func (l *logger) DebugEnabled() bool {
	return l.level >= LevelDebug
}
func (l *logger) TraceEnabled() bool {
	return l.level >= LevelTrace
}

func (l *logger) Error(v ...any) {
	if l.ErrorEnabled() {
		l.output(LevelError, v...)
	}
}
func (l *logger) Warn(v ...any) {
	if l.WarnEnabled() {
		l.output(LevelWarn, v...)
	}
}
func (l *logger) Info(v ...any) {
	if l.InfoEnabled() {
		l.output(LevelInfo, v...)
	}
}
func (l *logger) Debug(v ...any) {
	if l.DebugEnabled() {
		l.output(LevelDebug, v...)
	}
}
func (l *logger) Trace(v ...any) {
	if l.TraceEnabled() {
		l.output(LevelTrace, v...)
	}
}

func (l *logger) Errorf(format string, v ...any) {
	if l.ErrorEnabled() {
		l.outputf(LevelError, format, v...)
	}
}
func (l *logger) Warnf(format string, v ...any) {
	if l.WarnEnabled() {
		l.outputf(LevelWarn, format, v...)
	}
}
func (l *logger) Infof(format string, v ...any) {
	if l.InfoEnabled() {
		l.outputf(LevelInfo, format, v...)
	}
}
func (l *logger) Debugf(format string, v ...any) {
	if l.DebugEnabled() {
		l.outputf(LevelDebug, format, v...)
	}
}
func (l *logger) Tracef(format string, v ...any) {
	if l.TraceEnabled() {
		l.outputf(LevelTrace, format, v...)
	}
}

func (l *logger) output(level Level, v ...any) {
	_ = l.logger.Output(3, logHeader(level, 3)+fmt.Sprintln(v...))
}
func (l *logger) outputf(level Level, format string, v ...any) {
	_ = l.logger.Output(3, logHeader(level, 3)+fmt.Sprintf(format, v...))
}

func New(level Level, output io.Writer) *logger {
	return &logger{
		logger: log.New(output, "", 0),
		level:  level,
	}
}

func logHeader(level Level, callDepth int) string {
	now := time.Now().Format("2006-01-02T15:04:05.000Z07:00")
	padding := ""
	if len(level.String()) < 5 {
		padding = strings.Repeat(" ", 5-len(level.String()))
	}
	_, file, line, ok := runtime.Caller(callDepth)
	if ok {
		for i := len(file) - 2; i > 0; i-- {
			if file[i] == '/' {
				file = file[i+1:]
				break
			}
		}
	} else {
		file = "???"
	}
	return now + "|" + level.String() + padding + "| " + file + ":" + strconv.Itoa(line) + ": "
}
