// Package logger provides a high-performance, async, daily-rotating structured
// logging system for the CleverConnect VPN Orchestrator.
//
// Architecture:
//   - All log entries are dispatched to a buffered channel and written by a
//     dedicated background goroutine. This ensures zero impact on the hot path
//     of HTTP handlers, WebSocket streams, and database transactions.
//   - Log files rotate daily at midnight UTC and are stored in the ./logs/ directory.
//   - The standard library `log` package output is redirected into this system,
//     capturing third-party and package-level log calls (GORM, Gin, etc.).
//   - Supports structured fields (key=value) for rich, machine-parseable context.
//   - A graceful shutdown flush mechanism ensures no log entries are lost.
package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Level represents the severity of a log entry.
type Level int

const (
	DEBUG Level = iota
	INFO
	WARN
	ERROR
	FATAL
)

func (l Level) String() string {
	switch l {
	case DEBUG:
		return "DEBUG"
	case INFO:
		return "INFO"
	case WARN:
		return "WARN"
	case ERROR:
		return "ERROR"
	case FATAL:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}

// Entry represents a single structured log record.
type Entry struct {
	Time      time.Time
	Level     Level
	Component string // e.g. "Core", "HTTP", "WS", "DB", "Auth"
	Message   string
	Fields    map[string]interface{} // structured key=value context
	Caller    string                 // file:line of the call site
}

// Logger is the core async logging engine.
type Logger struct {
	mu        sync.Mutex
	dir       string                  // log output directory (./logs)
	file      *os.File                // current open daily log file handle
	fileDate  string                  // date string of the current file (YYYY-MM-DD)
	entries   chan *Entry             // buffered async channel
	done      chan struct{}           // shutdown signal
	wg        sync.WaitGroup
	minLevel  Level
	appMode   string                  // "client" or "server"
	listeners map[chan *Entry]bool    // active streaming log listeners
	listMu    sync.RWMutex            // read-write lock for listeners map
}

// global singleton – initialized once via Init()
var global *Logger

const (
	channelBuffer = 8192
	flushInterval = 500 * time.Millisecond
)

// RegisterListener registers a channel to receive clones of all emitted log entries.
func RegisterListener(ch chan *Entry) {
	if global == nil {
		return
	}
	global.listMu.Lock()
	defer global.listMu.Unlock()
	global.listeners[ch] = true
}

// UnregisterListener removes a registered log streaming listener.
func UnregisterListener(ch chan *Entry) {
	if global == nil {
		return
	}
	global.listMu.Lock()
	defer global.listMu.Unlock()
	delete(global.listeners, ch)
}

// GetTodayLogFilePath returns the path to the current daily log file.
func GetTodayLogFilePath() string {
	if global == nil {
		return ""
	}
	global.mu.Lock()
	defer global.mu.Unlock()
	today := time.Now().Format("2006-01-02")
	return filepath.Join(global.dir, fmt.Sprintf("%s_%s.log", global.appMode, today))
}

// Init creates the global logger, starts the background writer goroutine,
// and redirects the standard library log output into this system.
func Init(logDir string, appMode string) {
	if err := os.MkdirAll(logDir, 0755); err != nil {
		log.Fatalf("[Logger] Failed to create log directory %q: %v", logDir, err)
	}

	l := &Logger{
		dir:       logDir,
		entries:   make(chan *Entry, channelBuffer),
		done:      make(chan struct{}),
		minLevel:  DEBUG,
		appMode:   appMode,
		listeners: make(map[chan *Entry]bool),
	}

	// Open the initial file for today
	if err := l.rotateIfNeeded(); err != nil {
		log.Fatalf("[Logger] Failed to open initial log file: %v", err)
	}

	// Start the async writer
	l.wg.Add(1)
	go l.writer()

	global = l

	// Redirect the stdlib `log` package into our system so that third-party
	// packages (GORM, Gin Recovery, etc.) are captured automatically.
	log.SetOutput(&stdlibBridge{})
	log.SetFlags(0) // we handle timestamps ourselves

	global.Info("Logger", "Async structured logging system initialized",
		"dir", logDir,
		"mode", appMode,
		"buffer", channelBuffer,
	)
}

// Shutdown flushes all pending entries and closes the log file.
// Must be called on program exit (ideally via defer in main).
func Shutdown() {
	if global == nil {
		return
	}
	close(global.done)
	global.wg.Wait()

	global.mu.Lock()
	defer global.mu.Unlock()
	if global.file != nil {
		_ = global.file.Sync()
		_ = global.file.Close()
	}
}

// ---------------------------------------------------------------------------
// Public Logging API
// ---------------------------------------------------------------------------

func Debug(component, msg string, fields ...interface{}) {
	emitAt(DEBUG, component, msg, 4, fields...)
}

func Info(component, msg string, fields ...interface{}) {
	emitAt(INFO, component, msg, 4, fields...)
}

func Warn(component, msg string, fields ...interface{}) {
	emitAt(WARN, component, msg, 4, fields...)
}

func Error(component, msg string, fields ...interface{}) {
	emitAt(ERROR, component, msg, 4, fields...)
}

func Fatal(component, msg string, fields ...interface{}) {
	emitAt(FATAL, component, msg, 4, fields...)
	Shutdown()
	os.Exit(1)
}

// Instance methods (allow passing the logger explicitly if needed)

func (l *Logger) Debug(component, msg string, fields ...interface{}) {
	l.emitWithSkip(DEBUG, component, msg, 3, fields...)
}

func (l *Logger) Info(component, msg string, fields ...interface{}) {
	l.emitWithSkip(INFO, component, msg, 3, fields...)
}

func (l *Logger) Warn(component, msg string, fields ...interface{}) {
	l.emitWithSkip(WARN, component, msg, 3, fields...)
}

func (l *Logger) Error(component, msg string, fields ...interface{}) {
	l.emitWithSkip(ERROR, component, msg, 3, fields...)
}

func (l *Logger) Fatal(component, msg string, fields ...interface{}) {
	l.emitWithSkip(FATAL, component, msg, 3, fields...)
	Shutdown()
	os.Exit(1)
}

// ---------------------------------------------------------------------------
// Internal dispatch
// ---------------------------------------------------------------------------

func emitAt(level Level, component, msg string, skip int, fields ...interface{}) {
	if global == nil {
		// Fallback before Init(): print to stderr
		fmt.Fprintf(os.Stderr, "[%s] [%s] %s\n", level, component, msg)
		return
	}
	global.emitWithSkip(level, component, msg, skip, fields...)
}

func (l *Logger) emitWithSkip(level Level, component, msg string, skip int, fields ...interface{}) {
	if level < l.minLevel {
		return
	}

	e := &Entry{
		Time:      time.Now(),
		Level:     level,
		Component: component,
		Message:   msg,
		Caller:    caller(skip),
		Fields:    parseFields(fields...),
	}

	// Non-blocking send: if the channel is full, drop the oldest entry
	// to guarantee the hot path is never blocked.
	select {
	case l.entries <- e:
	default:
		// Channel full – evict one entry then push
		select {
		case <-l.entries:
		default:
		}
		l.entries <- e
	}

	// Also echo to stdout for real-time terminal visibility
	fmt.Print(l.format(e))
}

// ---------------------------------------------------------------------------
// Background writer goroutine
// ---------------------------------------------------------------------------

func (l *Logger) writer() {
	defer l.wg.Done()

	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	for {
		select {
		case entry := <-l.entries:
			l.write(entry)

		case <-ticker.C:
			// Periodic drain: flush all buffered entries and rotate if date changed
			l.drain()
			_ = l.rotateIfNeeded()

		case <-l.done:
			// Final drain on shutdown
			l.drain()
			return
		}
	}
}

func (l *Logger) drain() {
	for {
		select {
		case entry := <-l.entries:
			l.write(entry)
		default:
			return
		}
	}
}

func (l *Logger) write(e *Entry) {
	l.mu.Lock()
	defer l.mu.Unlock()

	_ = l.rotateIfNeeded()

	if l.file != nil {
		_, _ = l.file.WriteString(l.format(e))
	}

	// Broadcast to active log streaming listeners
	l.listMu.RLock()
	defer l.listMu.RUnlock()
	for ch := range l.listeners {
		select {
		case ch <- e:
		default:
			// Buffer full or slow consumer, drop to maintain zero-block guarantee
		}
	}
}

// ---------------------------------------------------------------------------
// Daily log file rotation
// ---------------------------------------------------------------------------

func (l *Logger) rotateIfNeeded() error {
	today := time.Now().Format("2006-01-02")
	if l.fileDate == today && l.file != nil {
		return nil
	}

	// Close the previous file
	if l.file != nil {
		_ = l.file.Sync()
		_ = l.file.Close()
	}

	filename := filepath.Join(l.dir, fmt.Sprintf("%s_%s.log", l.appMode, today))
	f, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open log file %q: %w", filename, err)
	}

	l.file = f
	l.fileDate = today

	// Write a rotation header
	header := fmt.Sprintf("\n══════════════════════════════════════════════════════════════════════\n"+
		"  CleverConnect %s — Log Session — %s\n"+
		"══════════════════════════════════════════════════════════════════════\n\n",
		strings.ToUpper(l.appMode), time.Now().Format("2006-01-02 15:04:05 MST"))
	_, _ = f.WriteString(header)

	return nil
}

// ---------------------------------------------------------------------------
// Entry formatting
// ---------------------------------------------------------------------------

func (l *Logger) format(e *Entry) string {
	var sb strings.Builder

	// Timestamp + Level + Component
	sb.WriteString(e.Time.Format("2006-01-02 15:04:05.000"))
	sb.WriteString(" │ ")
	sb.WriteString(padRight(e.Level.String(), 5))
	sb.WriteString(" │ ")
	sb.WriteString(padRight(e.Component, 8))
	sb.WriteString(" │ ")
	sb.WriteString(e.Message)

	// Structured fields
	if len(e.Fields) > 0 {
		sb.WriteString("  ‹")
		first := true
		for k, v := range e.Fields {
			if !first {
				sb.WriteString(", ")
			}
			fmt.Fprintf(&sb, "%s=%v", k, v)
			first = false
		}
		sb.WriteString("›")
	}

	// Caller info
	if e.Caller != "" {
		sb.WriteString("  @ ")
		sb.WriteString(e.Caller)
	}

	sb.WriteByte('\n')
	return sb.String()
}

// ---------------------------------------------------------------------------
// stdlib log.Writer bridge — captures third-party log output
// ---------------------------------------------------------------------------

type stdlibBridge struct{}

func (s *stdlibBridge) Write(p []byte) (n int, err error) {
	msg := strings.TrimSpace(string(p))
	if msg == "" {
		return len(p), nil
	}

	// Try to detect level and component from prefixed messages like "[DB] ..."
	component := "Vendor"
	level := INFO

	if strings.Contains(msg, "WARN") || strings.Contains(msg, "WARNING") {
		level = WARN
	} else if strings.Contains(msg, "ERR") || strings.Contains(msg, "FATAL") || strings.Contains(msg, "panic") {
		level = ERROR
	} else if strings.Contains(msg, "DEBUG") || strings.Contains(msg, "TRACE") {
		level = DEBUG
	}

	// Extract bracketed component tags like [DB], [WS], [Core]
	if len(msg) > 1 && msg[0] == '[' {
		if idx := strings.Index(msg, "]"); idx > 0 {
			component = msg[1:idx]
			msg = strings.TrimSpace(msg[idx+1:])
		}
	}

	emitAt(level, component, msg, 4)
	return len(p), nil
}

// ---------------------------------------------------------------------------
// GinWriter returns an io.Writer that routes Gin's internal logging (recovery
// panics, debug output) through our structured logger.
// ---------------------------------------------------------------------------

func GinWriter() io.Writer {
	return &ginBridge{}
}

type ginBridge struct{}

func (g *ginBridge) Write(p []byte) (n int, err error) {
	msg := strings.TrimSpace(string(p))
	if msg == "" {
		return len(p), nil
	}
	emitAt(ERROR, "GinRecov", msg, 4)
	return len(p), nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func caller(skip int) string {
	_, file, line, ok := runtime.Caller(skip)
	if !ok {
		return ""
	}
	// Use only the last two segments of the path for brevity
	parts := strings.Split(filepath.ToSlash(file), "/")
	if len(parts) > 2 {
		file = strings.Join(parts[len(parts)-2:], "/")
	}
	return fmt.Sprintf("%s:%d", file, line)
}

func parseFields(args ...interface{}) map[string]interface{} {
	if len(args) == 0 {
		return nil
	}
	fields := make(map[string]interface{}, len(args)/2)
	for i := 0; i+1 < len(args); i += 2 {
		key, ok := args[i].(string)
		if !ok {
			key = fmt.Sprintf("arg%d", i)
		}
		fields[key] = args[i+1]
	}
	return fields
}

func padRight(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}

// FormatEntry returns the standard printed string format of an Entry using the global formatter.
func FormatEntry(e *Entry) string {
	if global == nil {
		return ""
	}
	return global.format(e)
}
