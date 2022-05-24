package logger

import (
	"fmt"
	"io"
	"log"
)

type logger struct {
	debugLogger *log.Logger
	infoLogger  *log.Logger
	errorLogger *log.Logger
}

func (l *logger) Debug(v ...any) {
	if l.debugLogger != nil {
		_ = l.debugLogger.Output(2, fmt.Sprintln(v...))
	}
}

func (l *logger) Info(v ...any) {
	if l.infoLogger != nil {
		_ = l.infoLogger.Output(2, fmt.Sprintln(v...))
	}
}

func (l *logger) Error(v ...any) {
	if l.errorLogger != nil {
		_ = l.errorLogger.Output(2, fmt.Sprintln(v...))
	}
}

func (l *logger) Debugf(format string, v ...any) {
	if l.debugLogger != nil {
		_ = l.debugLogger.Output(2, fmt.Sprintf(format, v...))
	}
}

func (l *logger) Infof(format string, v ...any) {
	if l.infoLogger != nil {
		_ = l.infoLogger.Output(2, fmt.Sprintf(format, v...))
	}
}

func (l *logger) Errorf(format string, v ...any) {
	if l.errorLogger != nil {
		_ = l.errorLogger.Output(2, fmt.Sprintf(format, v...))
	}
}

func NewLogger(debugEnabled bool, infoEnabled bool, errorEnabled bool, output io.Writer) *logger {
	var (
		debugLogger *log.Logger
		infoLogger  *log.Logger
		errorLogger *log.Logger
	)

	flag := log.LstdFlags | log.Lshortfile

	if debugEnabled {
		debugLogger = log.New(output, "DEBUG:", flag)
	}
	if infoEnabled {
		infoLogger = log.New(output, "INFO :", flag)
	}
	if errorEnabled {
		errorLogger = log.New(output, "ERROR:", flag)
	}

	return &logger{
		debugLogger: debugLogger,
		infoLogger:  infoLogger,
		errorLogger: errorLogger,
	}
}
