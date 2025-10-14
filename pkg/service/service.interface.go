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

package service

import (
	"burlo/v2/pkg/logger"
	"context"
	"runtime/debug"
	"sync"
)

// Runnable is the common interface for all services.
type Runnable interface {
	Run(ctx context.Context)
}

func Start(ctx context.Context, ctxCancel context.CancelFunc, services []Runnable) <-chan int {
	wg := &sync.WaitGroup{}

	var exitCode int
	var exitCh = make(chan int, 1)

	log := logger.New("Panic")

	for _, s := range services {
		service := s
		wg.Go(func() {
			defer func() {
				if r := recover(); r != nil {
					log.Error("%v\n%s", r, debug.Stack())
					exitCode = -1
					ctxCancel()
				}
			}()
			service.Run(ctx)
		})
	}

	go func() {
		// wait for for all services to stop
		wg.Wait()
		exitCh <- exitCode
	}()

	return exitCh
}
