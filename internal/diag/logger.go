package diag

import (
	"fmt"
	"io"
	"strings"
	"sync"
)

// Level controls diagnostic log verbosity.
type Level int

const (
	LevelOff Level = iota
	LevelError
	LevelWarn
	LevelInfo
	LevelDebug
)

// ParseLevel normalizes a user-facing log level string.
func ParseLevel(raw string) (Level, error) {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case "", "off":
		return LevelOff, nil
	case "error":
		return LevelError, nil
	case "warn", "warning":
		return LevelWarn, nil
	case "info":
		return LevelInfo, nil
	case "debug":
		return LevelDebug, nil
	default:
		return LevelOff, fmt.Errorf("invalid --log-level value %q; expected off, error, warn, info, or debug", raw)
	}
}

// Logger writes diagnostic lines to stderr-safe output.
type Logger struct {
	level Level
	out   io.Writer
	mu    sync.Mutex
}

// NewLogger constructs a logger for the provided level and writer.
func NewLogger(out io.Writer, level Level) *Logger {
	if out == nil {
		out = io.Discard
	}
	return &Logger{
		level: level,
		out:   out,
	}
}

// Enabled reports whether a level should be emitted.
func (l *Logger) Enabled(level Level) bool {
	return l != nil && level != LevelOff && l.level >= level
}

// Errorf logs an error diagnostic line.
func (l *Logger) Errorf(scope, format string, args ...any) {
	l.logf(LevelError, scope, format, args...)
}

// Warnf logs a warning diagnostic line.
func (l *Logger) Warnf(scope, format string, args ...any) {
	l.logf(LevelWarn, scope, format, args...)
}

// Infof logs an informational diagnostic line.
func (l *Logger) Infof(scope, format string, args ...any) {
	l.logf(LevelInfo, scope, format, args...)
}

// Debugf logs a debug diagnostic line.
func (l *Logger) Debugf(scope, format string, args ...any) {
	l.logf(LevelDebug, scope, format, args...)
}

func (l *Logger) logf(level Level, scope, format string, args ...any) {
	if !l.Enabled(level) {
		return
	}

	var builder strings.Builder
	builder.WriteString("pituitary ")
	builder.WriteString(levelLabel(level))
	builder.WriteString(":")
	if scope = strings.TrimSpace(scope); scope != "" {
		builder.WriteString(" ")
		builder.WriteString(scope)
		builder.WriteString(":")
	}
	builder.WriteString(" ")
	builder.WriteString(fmt.Sprintf(format, args...))
	builder.WriteByte('\n')

	l.mu.Lock()
	defer l.mu.Unlock()
	_, _ = io.WriteString(l.out, builder.String())
}

func levelLabel(level Level) string {
	switch level {
	case LevelError:
		return "error"
	case LevelWarn:
		return "warn"
	case LevelInfo:
		return "info"
	case LevelDebug:
		return "debug"
	default:
		return "off"
	}
}
