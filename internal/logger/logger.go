package logger

import (
	"fmt"
	"log"
	"os"
	"strings"
	"sync/atomic"

	"github.com/pion/logging"
)

var verboseEnabled atomic.Bool //nolint:gochecknoglobals

var pionDropSubstrings = [...]string{
	"refresh permissions",
	"createpermission error response",
}

func DisableNoisyPionLogs() {
	mergePionLogDisable("turnc")
	removePionLogScopes([]string{"turnc"}, "ERROR", "WARN", "INFO", "DEBUG", "TRACE")
}

func mergePionLogDisable(scopes ...string) {
	const envKey = "PION_LOG_DISABLE"
	current := strings.TrimSpace(os.Getenv(envKey))
	if strings.EqualFold(current, "all") {
		return
	}
	seen := make(map[string]struct{}, len(scopes)+8)
	merged := make([]string, 0, len(scopes)+8)
	appendScope := func(raw string) {
		scope := strings.ToLower(strings.TrimSpace(raw))
		if scope == "" {
			return
		}
		if _, ok := seen[scope]; ok {
			return
		}
		seen[scope] = struct{}{}
		merged = append(merged, scope)
	}
	for _, scope := range strings.Split(current, ",") {
		appendScope(scope)
	}
	for _, scope := range scopes {
		appendScope(scope)
	}
	_ = os.Setenv(envKey, strings.Join(merged, ","))
}

func removePionLogScopes(scopes []string, levels ...string) {
	remove := make(map[string]struct{}, len(scopes))
	for _, scope := range scopes {
		scope = strings.ToLower(strings.TrimSpace(scope))
		if scope != "" {
			remove[scope] = struct{}{}
		}
	}
	if len(remove) == 0 {
		return
	}
	for _, level := range levels {
		envKey := "PION_LOG_" + level
		current := strings.TrimSpace(os.Getenv(envKey))
		if current == "" || strings.EqualFold(current, "all") {
			continue
		}
		parts := strings.Split(current, ",")
		kept := make([]string, 0, len(parts))
		for _, scope := range parts {
			scope = strings.ToLower(strings.TrimSpace(scope))
			if scope == "" {
				continue
			}
			if _, drop := remove[scope]; drop {
				continue
			}
			kept = append(kept, scope)
		}
		_ = os.Setenv(envKey, strings.Join(kept, ","))
	}
}

func SetVerbose(enabled bool) {
	verboseEnabled.Store(enabled)
}

func IsVerbose() bool {
	return verboseEnabled.Load()
}

func Info(v ...any) {
	log.Print(v...)
}

func Infof(format string, v ...any) {
	log.Printf(format, v...)
}

func Warn(v ...any) {
	log.Print(v...)
}

func Warnf(format string, v ...any) {
	log.Printf(format, v...)
}

func Error(v ...any) {
	log.Print(v...)
}

func Errorf(format string, v ...any) {
	log.Printf(format, v...)
}

func Verbosef(format string, v ...any) {
	if verboseEnabled.Load() {
		log.Printf(format, v...)
	}
}

func Debugf(format string, v ...any) {
	if verboseEnabled.Load() {
		log.Printf(format, v...)
	}
}

type PionLoggerFactory struct{}

func NewPionLoggerFactory() logging.LoggerFactory {
	return &PionLoggerFactory{}
}

func (f *PionLoggerFactory) NewLogger(scope string) logging.LeveledLogger {
	lower := strings.ToLower(scope)
	return &PionLeveledLogger{
		scope:     scope,
		dropScope: lower == "srtp" || lower == "turnc",
	}
}

type PionLeveledLogger struct {
	scope     string
	dropScope bool
}

func (l *PionLeveledLogger) Trace(msg string) {
	if verboseEnabled.Load() {
		log.Printf("[%s] TRACE: %s", l.scope, msg)
	}
}

func (l *PionLeveledLogger) Tracef(format string, args ...any) {
	if verboseEnabled.Load() {
		log.Printf("[%s] TRACE: %s", l.scope, fmt.Sprintf(format, args...))
	}
}

func (l *PionLeveledLogger) Debug(msg string) {
	if verboseEnabled.Load() {
		log.Printf("[%s] DEBUG: %s", l.scope, msg)
	}
}

func (l *PionLeveledLogger) Debugf(format string, args ...any) {
	if verboseEnabled.Load() {
		log.Printf("[%s] DEBUG: %s", l.scope, fmt.Sprintf(format, args...))
	}
}

func (l *PionLeveledLogger) Info(msg string) {
	if l.dropScope || dropPionMsg(msg) {
		return
	}
	log.Printf("[%s] INFO: %s", l.scope, msg)
}

func (l *PionLeveledLogger) Infof(format string, args ...any) {
	if l.dropScope {
		return
	}
	msg := fmt.Sprintf(format, args...)
	if dropPionMsg(msg) {
		return
	}
	log.Printf("[%s] INFO: %s", l.scope, msg)
}

func (l *PionLeveledLogger) Warn(msg string) {
	if l.dropScope || dropPionMsg(msg) {
		return
	}
	log.Printf("[%s] WARN: %s", l.scope, msg)
}

func (l *PionLeveledLogger) Warnf(format string, args ...any) {
	if l.dropScope {
		return
	}
	msg := fmt.Sprintf(format, args...)
	if dropPionMsg(msg) {
		return
	}
	log.Printf("[%s] WARN: %s", l.scope, msg)
}

func (l *PionLeveledLogger) Error(msg string) {
	if l.dropScope || dropPionMsg(msg) {
		return
	}
	log.Printf("[%s] ERROR: %s", l.scope, msg)
}

func (l *PionLeveledLogger) Errorf(format string, args ...any) {
	if l.dropScope {
		return
	}
	msg := fmt.Sprintf(format, args...)
	if dropPionMsg(msg) {
		return
	}
	log.Printf("[%s] ERROR: %s", l.scope, msg)
}

func shouldDropPionLog(scope, msg string) bool {
	if strings.EqualFold(scope, "srtp") || strings.EqualFold(scope, "turnc") {
		return true
	}
	return dropPionMsg(msg)
}

func dropPionMsg(msg string) bool {
	for _, sub := range pionDropSubstrings {
		if containsFold(msg, sub) {
			return true
		}
	}
	return false
}

func containsFold(s, sub string) bool {
	n := len(sub)
	if n == 0 {
		return true
	}
	limit := len(s) - n
	if limit < 0 {
		return false
	}
	first := sub[0]
	for i := 0; i <= limit; i++ {
		if !equalByteFold(s[i], first) {
			continue
		}
		j := 1
		for ; j < n; j++ {
			if !equalByteFold(s[i+j], sub[j]) {
				break
			}
		}
		if j == n {
			return true
		}
	}
	return false
}

func equalByteFold(a, b byte) bool {
	if a == b {
		return true
	}
	if 'A' <= a && a <= 'Z' {
		a += 'a' - 'A'
	}
	if 'A' <= b && b <= 'Z' {
		b += 'a' - 'A'
	}
	return a == b
}
