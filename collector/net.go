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
	net.Conn
	availableTransferTime time.Duration
	maxTransferTime       time.Duration
}

func NewConnReader(conn net.Conn, transferTime time.Duration) *ConnReader {
	return &ConnReader{Conn: conn, availableTransferTime: transferTime, maxTransferTime: transferTime}
}

func (c *ConnReader) Read(p []byte) (n int, err error) {
	if c.maxTransferTime > 0 {
		ch := make(chan struct{})
		if c.availableTransferTime <= 0 {
			return 0, fmt.Errorf("transfer timeout")
		}
		startTime := time.Now()
		// Transfer Timer
		defer func() {
			timeDelta := time.Now().Sub(startTime)
			for {
				oldTime := atomic.LoadInt64((*int64)(&c.availableTransferTime))
				newTime := oldTime - int64(timeDelta)
				if atomic.CompareAndSwapInt64((*int64)(&c.availableTransferTime), oldTime, newTime) {
					break
				}
			}
			close(ch)
		}()
		go func() {
			timer := time.NewTimer(time.Duration(atomic.LoadInt64((*int64)(&c.availableTransferTime))))
			select {
			case <-timer.C:
				c.Close()
			case <-ch:
				timer.Stop()
			}
		}()
	}
	data, err := c.Conn.Read(p)
	if err != nil {
		return data, io.EOF
	}
	return data, nil
}

func NewNetConfig(protocol string) *NetConfig {
	return &NetConfig{protocol: protocol}
}

type NetConfig struct {
	protocol        string
	Send            SendConfigs       `yaml:"send,omitempty"`
	MaxTransferTime *time.Duration    `yaml:"max_transfer_time"`
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
		t.MaxConnectTime = time.Second * 3
	}
	if t.protocol == "udp" && t.TLSConfig != nil {
		return fmt.Errorf("unknown protocol: tls over %s ", t.protocol)
	}
	if t.protocol != "udp" && t.protocol != "tcp" {
		return fmt.Errorf("unknown protocol: %s", t.protocol)
	}
	return nil
}

func (t NetConfig) GetStream(ctx context.Context, _, targetURL string) (io.ReadCloser, error) {
	logger, ok := ctx.Value(LoggerContextName).(log.Logger)
	if !ok {
		logger = log.NewNopLogger()
	}
	var conn net.Conn
	var err error
	if t.TLSConfig != nil {
		var tlsConfig *tls.Config
		tlsConfig, err = config.NewTLSConfig(t.TLSConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to load tls config: %s", err)
		}
		if t.protocol == "udp" {
			err = fmt.Errorf("unknown protocol: tls over %s ", t.protocol)
		} else if t.protocol == "tcp" {
			d := tls.Dialer{NetDialer: &net.Dialer{Timeout: t.MaxConnectTime}, Config: tlsConfig}
			conn, err = d.DialContext(ctx, t.protocol, targetURL)
		} else {
			err = fmt.Errorf("unknown protocol: %s", t.protocol)
		}
	} else {
		d := net.Dialer{Timeout: t.MaxConnectTime}
		conn, err = d.DialContext(ctx, t.protocol, targetURL)
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
	return NewConnReader(conn, *t.MaxTransferTime), nil
}

type SendConfig struct {
	Msg   string        `yaml:"msg,omitempty"`
	Delay time.Duration `yaml:"delay,omitempty"`
}

func (s *SendConfig) UnmarshalYAML(value *yaml.Node) error {
	if value.ShortTag() == "!!str" {
		var v string
		if err := value.Decode(&v); err != nil {
			return err
		}
		s.Msg = v
		return nil
	} else {
		type plain SendConfig
		return value.Decode((*plain)(s))
	}
}

type SendConfigs []SendConfig

func (s *SendConfigs) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.SequenceNode {
		type plain SendConfigs
		return value.Decode((*plain)(s))
	} else if value.Kind == yaml.MappingNode || value.ShortTag() == "!!str" {
		c := SendConfig{}
		if err := value.Decode(&c); err != nil {
			return err
		}
		*s = append(*s, c)
		return nil
	} else if value.Kind == yaml.AliasNode {
		return value.Alias.Decode(s)
	} else {
		return fmt.Errorf("unsupport type, expected map, list, or string, position: Line: %d,Column:%d", value.Line, value.Column)
	}
}
