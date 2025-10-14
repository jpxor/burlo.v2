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
	"bufio"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
)

// Service implements http.Handler for debug/log control
type Service struct {
	mu sync.Mutex
}

func WebService() *Service {
	return &Service{}
}

// ServeHTTP implements http.Handler
func (s *Service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/toggle":
		EnableDebug(!IsDebug())
		http.Redirect(w, r, "/logger", http.StatusSeeOther)

	case "/clear":
		if err := s.clearLog(); err != nil {
			http.Error(w, "failed to clear log: "+err.Error(), 500)
			return
		}
		http.Redirect(w, r, "/logger", http.StatusSeeOther)

	default:
		s.renderPage(w, r)
	}
}

func (s *Service) renderPage(w http.ResponseWriter, _ *http.Request) {
	logs, _ := s.tail(250) // last 250 lines

	tpl := `
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>Logger Service</title>
  <style>
    body { font-family: Arial, sans-serif; margin: 2em; background: #f9f9f9; color: #333; }
    h1 { margin-bottom: 0.5em; }
    .status { margin-bottom: 1em; }
    .btn { display:inline-block; padding:0.5em 1em; margin:0.2em; font-size:0.9em;
           background:#007bff; color:white; border:none; border-radius:4px; cursor:pointer; text-decoration:none; }
    .btn:hover { background:#0056b3; }
    .btn-danger { background:#dc3545; }
    .btn-danger:hover { background:#a71d2a; }
    pre.log { background:#222; color:#eee; padding:1em; border-radius:6px; max-height:500px; overflow:auto; }
  </style>
</head>
<body>
  <h1>Logger</h1>
  <div class="status">
    <b>Debug:</b> {{if .Debug}}<span style="color:green;">ON</span>{{else}}<span style="color:red;">OFF</span>{{end}}
  </div>
  <form method="POST" action="/logger/toggle" style="display:inline;">
    <button class="btn" type="submit">Toggle Debug</button>
  </form>
  <form method="POST" action="/logger/clear" style="display:inline;">
    <button class="btn btn-danger" type="submit">Clear Log</button>
  </form>
  <h2>Last 250 log lines</h2>
  <pre class="log">{{.Log}}</pre>
</body>
</html>
`
	t := template.Must(template.New("page").Parse(tpl))
	_ = t.Execute(w, map[string]any{
		"Debug": IsDebug(),
		"Log":   logs,
	})
}

// clearLog truncates and reopens the log file, rebuilding baseLogger
func (s *Service) clearLog() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if logFile == nil {
		return nil
	}

	// Close old file
	name := logFile.Name()
	logFile.Close()

	// Truncate file
	f, err := os.OpenFile(name, os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	f.Close()

	// Reopen and replace globals
	newf, err := os.OpenFile(name, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	logFile = newf

	// rebuild baseLogger with stdout + new file
	mw := io.MultiWriter(os.Stdout, logFile)
	baseLogger = newBaseLogger(mw)

	return nil
}

// tail reads last n lines of the log file
func (s *Service) tail(n int) (string, error) {
	if logFile == nil {
		return "", nil
	}
	f, err := os.Open(logFile.Name())
	if err != nil {
		return "", err
	}
	defer f.Close()

	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	// Join with newlines so each appears properly
	return strings.Join(lines, "\n"), sc.Err()
}

// helper to create baseLogger (keeps same flags)
func newBaseLogger(w io.Writer) *log.Logger {
	return log.New(w, "", log.LstdFlags)
}
