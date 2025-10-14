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

package appctx

import (
	"burlo/v2/pkg/logger"
	"context"
	"os"
	"os/signal"
	"syscall"
)

// WithSignal returns a context that is canceled when an OS signal (SIGINT or SIGTERM) is received.
// It also returns a cancel function you can call manually.
func New() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log := logger.New("SigHandler")
		sig := <-sigs
		log.Info("Received signal: %s\n", sig)
		cancel()
	}()

	return ctx, cancel
}
