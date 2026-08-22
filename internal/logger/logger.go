// Package logger provides a small leveled logger.
// Chat output must never go through this package; it is for diagnostics only.
package logger

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	default:
		return "INFO"
	}
}

// Logger is a mutex-protected, leveled logger. Zero value is not useful;
// construct with New.
type Logger struct {
	mu    sync.Mutex
	out   io.Writer
	debug bool
}

func New(out io.Writer, debug bool) *Logger {
	if out == nil {
		out = io.Discard
	}
	return &Logger{out: out, debug: debug}
}

func NewDefault(debug bool) *Logger {
	return New(os.Stderr, debug)
}

func (l *Logger) logf(level Level, format string, args ...any) {
	if l == nil {
		return
	}
	if level == LevelDebug && !l.debug {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	ts := time.Now().Format("15:04:05")
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(l.out, "%s [%s] %s\n", ts, level.String(), msg)
}

func (l *Logger) Debugf(format string, args ...any) { l.logf(LevelDebug, format, args...) }
func (l *Logger) Infof(format string, args ...any)  { l.logf(LevelInfo, format, args...) }
func (l *Logger) Warnf(format string, args ...any)  { l.logf(LevelWarn, format, args...) }
func (l *Logger) Errorf(format string, args ...any) { l.logf(LevelError, format, args...) }
