// Copyright 2021 MicroOps
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package collector

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/common/config"
	"gopkg.in/yaml.v3"
	"io"
	"net"
	"sync/atomic"
	"time"
)

type ConnReader struct {
	io.ReadWriteCloser
	buf                   *bufio.Scanner
	lineBuf               []byte
	availableTransferTime time.Duration
}

func NewConnReader(conn io.ReadWriteCloser, transferTime time.Duration, endOf []byte) *ConnReader {
	reader := &ConnReader{ReadWriteCloser: conn, buf: bufio.NewScanner(conn), availableTransferTime: transferTime}
	reader.buf.Split(func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		if atEOF && len(data) == 0 {
			return 0, nil, nil
		}
		if len(endOf) > 0 {
			if i := bytes.Index(data, endOf); i >= 0 {
				return i + len(endOf), data[0 : i+len(endOf)], bufio.ErrFinalToken
			}
		}

		if i := bytes.IndexByte(data, '\n'); i >= 0 {
			// We have a full newline-terminated line.
			return i + 1, data[0 : i+1], nil
		}
		// If we're at EOF, we have a final, non-terminated line. Return it.
		if atEOF {
			return len(data), data, nil
		}
		// Request more data.
		return 0, nil, nil
	})
	return reader
}

func (c *ConnReader) Read(p []byte) (n int, err error) {
	ch := make(chan struct{})
	if c.availableTransferTime <= 0 {
		return 0, fmt.Errorf("transfer timeout")
	}
	startTime := time.Now()
	defer func() {
		timeDelta := time.Now().Sub(startTime)
		for {
			oldBits := atomic.LoadInt64((*int64)(&c.availableTransferTime))
			newBits := oldBits - int64(timeDelta)
			if atomic.CompareAndSwapInt64((*int64)(&c.availableTransferTime), oldBits, newBits) {
				break
			}
		}
		close(ch)
	}()
	if c.buf.Err() != nil {
		return 0, c.buf.Err()
	}
	if len(c.lineBuf) == 0 {
		go func() {
			timer := time.NewTimer(time.Duration(atomic.LoadInt64((*int64)(&c.availableTransferTime))))
			select {
			case <-timer.C:
				c.Close()
			case <-ch:
			}
		}()
		if !c.buf.Scan() {
			return 0, io.EOF
		}
		c.lineBuf = c.buf.Bytes()
	}
	n = copy(p, c.lineBuf)
	c.lineBuf = c.lineBuf[n:]
	return n, nil
}

type NetConfig struct {
	Send            SendConfigs       `yaml:"send,omitempty"`
	MaxTransferTime time.Duration     `yaml:"max_transfer_time"`
	MaxConnectTime  time.Duration     `yaml:"max_connect_time"`
	TLSConfig       *config.TLSConfig `yaml:"tls_config,omitempty" json:"tls_config,omitempty"`
	EndOf           string            `yaml:"end_of"`
}

func (t *NetConfig) UnmarshalYAML(value *yaml.Node) error {
	type plain NetConfig
	if err := value.Decode((*plain)(t)); err != nil {
		return err
	}
	if t.MaxConnectTime == 0 {
		t.MaxTransferTime = time.Second * 3
	}
	if t.MaxTransferTime == 0 {
		t.MaxTransferTime = time.Second * 3
	}
	return nil
}

func (t NetConfig) GetStream(ctx context.Context, _, targetURL, protocol string) (io.ReadCloser, error) {
	logger, ok := ctx.Value(LoggerContextName).(log.Logger)
	if !ok {
		logger = log.NewNopLogger()
	}
	var conn io.ReadWriteCloser
	var err error
	if t.TLSConfig != nil {
		var tlsConfig *tls.Config
		tlsConfig, err = config.NewTLSConfig(t.TLSConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to load tls config: %s", err)
		}
		if protocol == "udp" {
			err = fmt.Errorf("unknown protocol: tls over %s ", protocol)
		} else if protocol == "tcp" {
			d := tls.Dialer{NetDialer: &net.Dialer{Timeout: t.MaxConnectTime}, Config: tlsConfig}
			conn, err = d.DialContext(ctx, protocol, targetURL)
		} else {
			err = fmt.Errorf("unknown protocol: %s", protocol)
		}
	} else {
		d := net.Dialer{Timeout: t.MaxConnectTime}
		conn, err = d.DialContext(ctx, protocol, targetURL)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to connect to server: %s", err)
	}
	if len(t.Send) > 0 {
		go func() {
			for _, sendConfig := range t.Send {
				if ctx.Err() != nil {
					break
				}
				if _, err = conn.Write([]byte(sendConfig.Msg)); err != nil {
					level.Error(logger).Log("msg", "failed to send msg")
					return
				} else if sendConfig.Delay > 0 {
					time.Sleep(sendConfig.Delay)
				}
			}
			//<-ctx.Done()
			//conn.Close()
		}()
	}
	return NewConnReader(conn, t.MaxTransferTime, []byte(t.EndOf)), nil
}
