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

package modbus

import (
	"burlo/v2/pkg/logger"
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	wrapper "github.com/grid-x/modbus"
)

type Client struct {
	mu      sync.Mutex
	handler *wrapper.TCPClientHandler
	client  wrapper.Client
	config  *Config
	log     *logger.Logger
	ctx     context.Context
}

// NewClient creates and connects a Modbus TCP client with sane defaults.
func NewClient(ctx context.Context, config *Config) *Client {
	log := logger.New("ModbusConn")

	c := &Client{
		config: config,
		log:    log,
		ctx:    ctx,
	}
	if err := c.connectWithRetry(); err != nil {
		log.Fatal("failed to connect to modbus device: %v", err)
	}
	return c
}

// connectWithRetry tries to connect, retrying indefinitely until success.
func (c *Client) connectWithRetry() error {
	backoff := time.Second
	for {
		if err := c.connect(); err != nil {
			c.log.Error("Modbus connect failed: %v (retrying in %v)", err, backoff)
			time.Sleep(backoff)

			// exponential backoff up to 30 seconds
			if backoff < 30*time.Second {
				backoff *= 2
				if backoff > 30*time.Second {
					backoff = 30 * time.Second
				}
			}
			continue
		}
		return nil
	}
}

// connect safely (re)connects the Modbus client once.
func (c *Client) connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.handler != nil {
		_ = c.handler.Close()
	}

	url := fmt.Sprintf("%s:%d", c.config.Modbus.Host, c.config.Modbus.Port)
	handler := wrapper.NewTCPClientHandler(url)
	handler.SlaveID = c.config.Modbus.SlaveID
	handler.Timeout = time.Second * time.Duration(c.config.Modbus.Timeout)
	handler.ProtocolRecoveryTimeout = 250 * time.Millisecond
	handler.LinkRecoveryTimeout = 5 * time.Second

	c.log.Info("Connecting to %s...", url)
	if err := handler.Connect(c.ctx); err != nil {
		return fmt.Errorf("modbus connect failed: %w", err)
	}

	c.handler = handler
	c.client = wrapper.NewClient(handler)
	c.log.Info("Connected to %s", url)
	return nil
}

// retry wraps Modbus operations and reconnects automatically if needed.
func (c *Client) retry(op func() error) error {
	var err error
	for attempt := 0; attempt < 2; attempt++ {
		err = op()
		if err == nil {
			return nil
		}
		if !isConnError(err) {
			c.log.Debug("retry after err: %+v", err)
			continue
		}

		c.log.Error("connection error: %v — reconnecting...", err)
		c.connectWithRetry() // blocks until connected
	}
	c.log.Error("too many retries: %+v", err)
	c.log.Error("will attemp reconnect")
	c.connectWithRetry()
	return err
}

// WriteRegister writes a single holding register safely, retrying if needed.
func (c *Client) WriteRegister(ctx context.Context, addr, value uint16) error {
	return c.retry(func() error {
		c.mu.Lock()
		defer c.mu.Unlock()
		_, err := c.client.WriteSingleRegister(ctx, addr, value)
		return err
	})
}

// ReadRegisters reads holding registers safely, retrying if needed.
func (c *Client) ReadRegisters(ctx context.Context, addr, quantity uint16) ([]byte, error) {
	var data []byte
	err := c.retry(func() error {
		c.mu.Lock()
		defer c.mu.Unlock()
		var rerr error
		data, rerr = c.client.ReadHoldingRegisters(ctx, addr, quantity)
		return rerr
	})
	return data, err
}

// Close closes the underlying handler.
func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.handler != nil {
		_ = c.handler.Close()
	}
}

// --- helpers ---

func isConnError(err error) bool {
	if err == nil {
		return false
	}
	var nerr net.Error
	if errors.As(err, &nerr) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "closed by the remote host") ||
		strings.Contains(msg, "i/o timeout") ||
		strings.Contains(msg, "use of closed network connection") ||
		strings.Contains(msg, "connection refused")
}
