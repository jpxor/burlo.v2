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

package phidgets

import (
	"burlo/v2/internal/config"
	"burlo/v2/pkg/logger"
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"time"
)

// Path to your Python service script
const pythonScript = "internal/phidgets/phidgets.service.py"

type Manager struct {
	conf *config.Config
	log  *logger.Logger
}

func New(conf *config.Config) *Manager {
	return &Manager{
		conf: conf,
		log:  logger.New("Phidgets  "),
	}
}

func (m *Manager) Run(ctx context.Context) {
	for {
		// If context is canceled, exit loop (shutdown requested)
		select {
		case <-ctx.Done():
			m.log.Info("Stopped")
			return
		default:
		}
		cmd := exec.CommandContext(ctx, "python3", pythonScript, m.conf.Phidgets.HTTPAddr)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		m.log.Info("Running...")

		err := cmd.Start()
		if err != nil {
			m.log.Error("Failed to start python cmd: %v", err)
			time.Sleep(5 * time.Second)
			continue
		}

		// Wait until it stops or context is canceled
		err = cmd.Wait()

		if err != nil {
			m.log.Error("cmd exited with error: %v", err)
		}
		m.log.Info("Restarting")
		time.Sleep(2 * time.Second)
	}
}

func (m *Manager) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	url := fmt.Sprintf("http://%s%s/phidgets/state", r.Host, m.conf.Phidgets.HTTPAddr)
	http.Redirect(w, r, url, http.StatusFound)
}
