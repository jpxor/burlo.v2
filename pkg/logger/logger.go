// Copyright (C) 2025 Josh Simonot
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

type Logger struct {
	prefix string
	logger *log.Logger
}

var (
	baseLogger   *log.Logger
	logFile      *os.File
	once         sync.Once
	debugEnabled bool
	debugMu      sync.RWMutex
)

// Init initializes the base logger with stdout and a log file.
// Optionally enables debug if DEBUG env var is set.
func Init(logPath string) error {
	var err error
	once.Do(func() {
		logFile, err = os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return
		}

		mw := io.MultiWriter(os.Stdout, logFile)
		baseLogger = log.New(mw, "", log.LstdFlags)

		// enable debug from env at startup if wanted
		if os.Getenv("DEBUG") != "" {
			debugEnabled = true
		}
	})
	return err
}

// Close cleans up the log file (call on shutdown)
func Close() {
	if logFile != nil {
		logFile.Close()
	}
}

// EnableDebug dynamically turns debug logging on/off
func EnableDebug(on bool) {
	debugMu.Lock()
	debugEnabled = on
	debugMu.Unlock()
}

// IsDebug returns current debug state
func IsDebug() bool {
	debugMu.RLock()
	defer debugMu.RUnlock()
	return debugEnabled
}

func New(prefix string) *Logger {
	Init("default.log")
	return &Logger{
		prefix: prefix,
		logger: log.New(baseLogger.Writer(), "", log.LstdFlags),
	}
}

func (l *Logger) Info(fmtstr string, v ...any) {
	formatted := fmt.Sprintf(fmtstr, v...)
	l.logger.Printf("[%s] INFO: %v", l.prefix, formatted)
}

func (l *Logger) Error(fmtstr string, v ...any) {
	formatted := fmt.Sprintf(fmtstr, v...)
	_, file, line, ok := runtime.Caller(1)
	if ok {
		file = filepath.Base(file)
		l.logger.Printf("[%s] ERROR: (%s:%d) %s", l.prefix, file, line, formatted)
	} else {
		l.logger.Printf("[%s] ERROR: %v", l.prefix, formatted)
	}
}

func (l *Logger) Fatal(fmtstr string, v ...any) {
	formatted := fmt.Sprintf(fmtstr, v...)
	_, file, line, ok := runtime.Caller(1)
	if ok {
		file = filepath.Base(file)
		l.logger.Printf("[%s] FATAL: (%s:%d) %s", l.prefix, file, line, formatted)
	} else {
		l.logger.Printf("[%s] FATAL: %v", l.prefix, formatted)
	}
	panic(formatted)
}

func (l *Logger) Debug(fmtstr string, v ...any) {
	debugMu.RLock()
	enabled := debugEnabled
	debugMu.RUnlock()
	if !enabled {
		return
	}
	formatted := fmt.Sprintf(fmtstr, v...)
	l.logger.Printf("[%s] DEBUG: %v", l.prefix, formatted)
}
