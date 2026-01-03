// Package logger provides structured logging functionality for Alem Community Hub.
// It supports log levels, structured fields, and context propagation.
// No external dependencies - uses only standard library.
package logger

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Level represents the severity of a log message.
type Level int

const (
	// LevelDebug is for detailed debugging information.
	LevelDebug Level = iota
	// LevelInfo is for general operational information.
	LevelInfo
	// LevelWarn is for warning messages.
	LevelWarn
	// LevelError is for error messages.
	LevelError
	// LevelFatal is for fatal errors that require program termination.
	LevelFatal
)

// String returns the string representation of the log level.
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
	case LevelFatal:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}

// ParseLevel parses a string into a Level.
func ParseLevel(s string) Level {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "DEBUG":
		return LevelDebug
	case "INFO":
		return LevelInfo
	case "WARN", "WARNING":
		return LevelWarn
	case "ERROR":
		return LevelError
	case "FATAL":
		return LevelFatal
	default:
		return LevelInfo
	}
}

// Field represents a key-value pair for structured logging.
type Field struct {
	Key   string
	Value any
}

// F creates a new Field with the given key and value.
func F(key string, value any) Field {
	return Field{Key: key, Value: value}
}

// Common field constructors for convenience.
func String(key, value string) Field          { return Field{Key: key, Value: value} }
func Int(key string, value int) Field         { return Field{Key: key, Value: value} }
func Int64(key string, value int64) Field     { return Field{Key: key, Value: value} }
func Float64(key string, value float64) Field { return Field{Key: key, Value: value} }
func Bool(key string, value bool) Field       { return Field{Key: key, Value: value} }

// Err creates an error field.
func Err(err error) Field {
	if err == nil {
		return Field{Key: "error", Value: nil}
	}
	return Field{Key: "error", Value: err.Error()}
}

// Duration creates a duration field.
func Duration(key string, value time.Duration) Field {
	return Field{Key: key, Value: value.String()}
}

// Time creates a time field.
func Time(key string, value time.Time) Field {
	return Field{Key: key, Value: value.Format(time.RFC3339)}
}

// Any creates a field with any value.
func Any(key string, value any) Field { return Field{Key: key, Value: value} }

// LogEntry represents a single log entry.
type LogEntry struct {
	Timestamp string         `json:"timestamp"`
	Level     string         `json:"level"`
	Message   string         `json:"message"`
	Caller    string         `json:"caller,omitempty"`
	Fields    map[string]any `json:"fields,omitempty"`
}

// Logger is the main logger struct.
type Logger struct {
	mu         sync.Mutex
	output     io.Writer
	level      Level
	fields     []Field
	addCaller  bool
	callerSkip int
}

// Options configures the logger.
type Options struct {
	Output     io.Writer
	Level      Level
	AddCaller  bool
	CallerSkip int
}

// DefaultOptions returns sensible defaults for the logger.
func DefaultOptions() Options {
	return Options{
		Output:     os.Stdout,
		Level:      LevelInfo,
		AddCaller:  true,
		CallerSkip: 0,
	}
}

// New creates a new Logger with the given options.
func New(opts Options) *Logger {
	if opts.Output == nil {
		opts.Output = os.Stdout
	}
	return &Logger{
		output:     opts.Output,
		level:      opts.Level,
		addCaller:  opts.AddCaller,
		callerSkip: opts.CallerSkip,
		fields:     make([]Field, 0),
	}
}

// Default creates a logger with default options.
func Default() *Logger {
	return New(DefaultOptions())
}

// With returns a new Logger with the given fields added.
func (l *Logger) With(fields ...Field) *Logger {
	newLogger := &Logger{
		output:     l.output,
		level:      l.level,
		addCaller:  l.addCaller,
		callerSkip: l.callerSkip,
		fields:     make([]Field, len(l.fields)+len(fields)),
	}
	copy(newLogger.fields, l.fields)
	copy(newLogger.fields[len(l.fields):], fields)
	return newLogger
}

// WithLevel returns a new Logger with the specified minimum log level.
func (l *Logger) WithLevel(level Level) *Logger {
	return &Logger{
		output:     l.output,
		level:      level,
		addCaller:  l.addCaller,
		callerSkip: l.callerSkip,
		fields:     l.fields,
	}
}

// log is the internal logging method.
func (l *Logger) log(level Level, msg string, fields ...Field) {
	if level < l.level {
		return
	}

	entry := LogEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Level:     level.String(),
		Message:   msg,
	}

	// Add caller information if enabled
	if l.addCaller {
		_, file, line, ok := runtime.Caller(2 + l.callerSkip)
		if ok {
			// Shorten the file path
			if idx := strings.LastIndex(file, "/"); idx >= 0 {
				file = file[idx+1:]
			}
			entry.Caller = fmt.Sprintf("%s:%d", file, line)
		}
	}

	// Merge base fields and additional fields
	allFields := make([]Field, 0, len(l.fields)+len(fields))
	allFields = append(allFields, l.fields...)
	allFields = append(allFields, fields...)

	if len(allFields) > 0 {
		entry.Fields = make(map[string]any, len(allFields))
		for _, f := range allFields {
			entry.Fields[f.Key] = f.Value
		}
	}

	// Marshal and write
	l.mu.Lock()
	defer l.mu.Unlock()

	data, err := json.Marshal(entry)
	if err != nil {
		// Fallback to simple format on marshal error
		fmt.Fprintf(l.output, "%s [%s] %s\n", entry.Timestamp, entry.Level, msg)
		return
	}

	l.output.Write(data)
	l.output.Write([]byte("\n"))
}

// Debug logs a debug message.
func (l *Logger) Debug(msg string, fields ...Field) {
	l.log(LevelDebug, msg, fields...)
}

// Info logs an info message.
func (l *Logger) Info(msg string, fields ...Field) {
	l.log(LevelInfo, msg, fields...)
}

// Warn logs a warning message.
func (l *Logger) Warn(msg string, fields ...Field) {
	l.log(LevelWarn, msg, fields...)
}

// Error logs an error message.
func (l *Logger) Error(msg string, fields ...Field) {
	l.log(LevelError, msg, fields...)
}

// Fatal logs a fatal message and exits the program.
func (l *Logger) Fatal(msg string, fields ...Field) {
	l.log(LevelFatal, msg, fields...)
	os.Exit(1)
}

// Debugf logs a formatted debug message.
func (l *Logger) Debugf(format string, args ...any) {
	l.log(LevelDebug, fmt.Sprintf(format, args...))
}

// Infof logs a formatted info message.
func (l *Logger) Infof(format string, args ...any) {
	l.log(LevelInfo, fmt.Sprintf(format, args...))
}

// Warnf logs a formatted warning message.
func (l *Logger) Warnf(format string, args ...any) {
	l.log(LevelWarn, fmt.Sprintf(format, args...))
}

// Errorf logs a formatted error message.
func (l *Logger) Errorf(format string, args ...any) {
	l.log(LevelError, fmt.Sprintf(format, args...))
}

// Fatalf logs a formatted fatal message and exits.
func (l *Logger) Fatalf(format string, args ...any) {
	l.log(LevelFatal, fmt.Sprintf(format, args...))
	os.Exit(1)
}

// Context key for logger.
type ctxKey struct{}

// WithContext returns a new context with the logger attached.
func WithContext(ctx context.Context, l *Logger) context.Context {
	return context.WithValue(ctx, ctxKey{}, l)
}

// FromContext retrieves the logger from context, or returns a default logger.
func FromContext(ctx context.Context) *Logger {
	if l, ok := ctx.Value(ctxKey{}).(*Logger); ok {
		return l
	}
	return Default()
}

// RequestIDKey is a common field key for request tracing.
const RequestIDKey = "request_id"

// WithRequestID returns a logger with request ID field added.
func (l *Logger) WithRequestID(requestID string) *Logger {
	return l.With(String(RequestIDKey, requestID))
}

// Student-related logging helpers for Alem Hub.
func StudentID(id string) Field     { return String("student_id", id) }
func TelegramID(id int64) Field     { return Int64("telegram_id", id) }
func Email(email string) Field      { return String("email", email) }
func TaskName(name string) Field    { return String("task_name", name) }
func XPAmount(xp int) Field         { return Int("xp_amount", xp) }
func RankPosition(pos int) Field    { return Int("rank_position", pos) }
func Component(name string) Field   { return String("component", name) }
func Operation(name string) Field   { return String("operation", name) }
func Latency(d time.Duration) Field { return Duration("latency", d) }
