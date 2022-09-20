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
	"errors"
	"fmt"
	"github.com/MicroOps-cn/data_exporter/pkg/buffer"
	"github.com/go-kit/log"
	"github.com/hpcloud/tail"
	promconfig "github.com/prometheus/common/config"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"gopkg.in/yaml.v3"
	"io"
	stdlog "log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type DatasourceType string

const (
	Http  DatasourceType = "http"
	Https DatasourceType = "https"
	File  DatasourceType = "file"
	Tcp   DatasourceType = "tcp"
	Udp   DatasourceType = "udp"
)

func (d DatasourceType) ToLower() DatasourceType {
	return DatasourceType(strings.ToLower(string(d)))
}
func (d DatasourceType) ToLowerString() string {
	return strings.ToLower(string(d))
}

type DatasourceReadMode string

const (
	Line       DatasourceReadMode = "line"
	Stream     DatasourceReadMode = "stream"
	StreamLine DatasourceReadMode = "stream-line"
	FullText   DatasourceReadMode = "full-text"
	Full       DatasourceReadMode = "full"
)

func (d DatasourceReadMode) ToLower() DatasourceReadMode {
	return DatasourceReadMode(strings.ToLower(string(d)))
}

type HTTPConfig struct {
	HTTPClientConfig promconfig.HTTPClientConfig `yaml:"http_client_config,inline"`
	Body             string                      `yaml:"body,omitempty"`
	Headers          map[string]string           `yaml:"headers,omitempty"`
	Method           string                      `yaml:"method,omitempty"`
	ValidStatusCodes []int                       `yaml:"valid_status_codes,omitempty"`
	MaxConnectTime   time.Duration               `yaml:"max_connect_time"`
}

func (h *HTTPConfig) UnmarshalYAML(value *yaml.Node) error {
	type plain HTTPConfig
	err := value.Decode((*plain)(h))
	if err != nil {
		return err
	}
	if h.MaxConnectTime == time.Duration(0) {
		h.MaxConnectTime = DatasourceDefaultConnectTimeout
	}
	if h.MaxConnectTime < time.Millisecond {
		return fmt.Errorf("timeout value cannot be less than 1 ms: timeout=%s", h.MaxConnectTime)
	}
	return nil
}

func (h HTTPConfig) GetStream(ctx context.Context, name, targetURL string) (io.ReadCloser, error) {
	dialerFunc := func(ctx context.Context, network string, addr string) (net.Conn, error) {
		conn, err := net.DialTimeout(network, addr, h.MaxConnectTime)
		if err != nil {
			return nil, err
		}
		return conn, nil
	}
	client, err := promconfig.NewClientFromConfig(h.HTTPClientConfig, name, promconfig.WithKeepAlivesDisabled(), promconfig.WithDialContextFunc(dialerFunc))
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(targetURL, "http://") && !strings.HasPrefix(targetURL, "https://") {
		targetURL = "http://" + targetURL
	}
	var body io.Reader
	if h.Body != "" {
		body = strings.NewReader(h.Body)
	}
	request, err := http.NewRequest(h.Method, targetURL, body)
	if err != nil {
		return nil, err
	}
	for key, value := range h.Headers {
		if cases.Title(language.English).String(key) == "Host" {
			request.Host = value
			continue
		}

		request.Header.Set(key, value)
	}
	resp, err := client.Do(request.WithContext(ctx))
	if err != nil {
		return nil, err
	} else if resp.Body == nil {
		return nil, fmt.Errorf("response body is nil")
	}
	if len(h.ValidStatusCodes) != 0 {
		for _, code := range h.ValidStatusCodes {
			if resp.StatusCode == code {
				return resp.Body, nil
			}
		}
		return nil, fmt.Errorf("invalid HTTP response status code: %d not in %v", resp.StatusCode, h.ValidStatusCodes)
	} else if 200 <= resp.StatusCode && resp.StatusCode < 300 {
		return resp.Body, nil
	} else {
		return nil, fmt.Errorf("invalid HTTP response status code %d,wanted 2xx", resp.StatusCode)
	}
}

type Streamer interface {
	GetStream(ctx context.Context, name, targetURL string) (io.ReadCloser, error)
}

type Datasource struct {
	Name                 string             `yaml:"name,omitempty"`
	Url                  string             `yaml:"url"`
	AllowReplace         bool               `yaml:"allow_replace,omitempty"`
	Type                 DatasourceType     `yaml:"type"`
	Timeout              time.Duration      `yaml:"timeout"`
	RelabelConfigs       RelabelConfigs     `yaml:"relabel_configs,omitempty"`
	MaxContentLength     *int64             `yaml:"max_content_length"`
	MinContentLength     *int               `yaml:"min_content_length,omitempty"`
	LineMaxContentLength *int               `yaml:"line_max_content_length"`
	LineSeparator        buffer.SliceString `yaml:"line_separator"`
	r                    io.ReadCloser
	EndOf                string             `yaml:"end_of,omitempty"`
	ReadMode             DatasourceReadMode `yaml:"read_mode"`
	Config               Streamer           `yaml:"-"`
	Whence               int                `yaml:"whence"`

	// Deprecated
	HTTPConfig *HTTPConfig `yaml:"http,omitempty"`
	// Deprecated
	TCPConfig *NetConfig `yaml:"tcp,omitempty"`
	// Deprecated
	UDPConfig *NetConfig `yaml:"udp,omitempty"`
}

var (
	DefaultHttpConfig               = HTTPConfig{Method: "GET"}
	DatasourceDefaultTimeout        = time.Second * 30
	DatasourceDefaultConnectTimeout = time.Second * 3
)

const DefaultMaxContent = 102400000

var ErrorDataTooSort = errors.New("data is too short")

func (d *Datasource) MarshalYAML() (interface{}, error) {
	type plain Datasource
	type T struct {
		Config Streamer `yaml:"config"`
		*plain `yaml:",inline"`
	}
	var ds = T{
		plain:  (*plain)(d),
		Config: d.Config,
	}
	return &ds, nil
}

func (d *Datasource) UnmarshalYAML(value *yaml.Node) error {
	type plain Datasource
	type T struct {
		Config *struct{} `yaml:"config"`
		*plain `yaml:",inline"`
	}
	d.Whence = io.SeekEnd
	obj := &T{plain: (*plain)(d)}
	if err := value.Decode(obj); err != nil {
		return err
	} else {
		d.ReadMode = d.ReadMode.ToLower()
		switch d.ReadMode {
		case "", FullText:
			d.ReadMode = Full
		case StreamLine:
			d.ReadMode = Line
		case Stream, Line, Full:
		default:
			return fmt.Errorf("read_type value ( %s ) is error", d.ReadMode)
		}
		if d.Type == "" {
			if u, err := url.Parse(d.Url); err == nil {
				if u.Scheme != "" {
					d.Type = DatasourceType(u.Scheme)
				}
			}
		}
		switch d.Type {
		case File:
		case Http, Https:
			d.Type = Http
			if d.HTTPConfig != nil {
				d.Config = d.HTTPConfig
			} else if obj.Config != nil {
				httpConfig := new(HTTPConfig)
				if err = value.Decode(&struct {
					Config *HTTPConfig
				}{Config: httpConfig}); err != nil {
					return nil
				}
				d.Config = httpConfig
			} else {
				d.Config = &DefaultHttpConfig
			}
		case Tcp, Udp:
			if d.Type == Tcp && d.TCPConfig != nil {
				d.Config = d.TCPConfig
			} else if d.Type == Udp && d.UDPConfig != nil {
				d.Config = d.UDPConfig
			} else {
				netConfig := NewNetConfig(string(d.Type))
				if obj.Config != nil {
					if err = value.Decode(&struct {
						Config *NetConfig
					}{Config: netConfig}); err != nil {
						return nil
					}
				}
				d.Config = netConfig
			}
			if len(d.Config.(*NetConfig).EndOf) > 0 && len(d.EndOf) == 0 {
				d.EndOf = d.Config.(*NetConfig).EndOf
			}
			if d.Config.(*NetConfig).MaxTransferTime == nil {
				d.Config.(*NetConfig).MaxTransferTime = new(time.Duration)
				if d.ReadMode == Stream {
					*d.Config.(*NetConfig).MaxTransferTime = 0
				} else {
					*d.Config.(*NetConfig).MaxTransferTime = time.Second * 3
				}
			}
		default:
			return fmt.Errorf("Unknown datasource type: %s. ", d.Type)
		}
		if d.LineMaxContentLength == nil {
			d.LineMaxContentLength = new(int)
			*d.LineMaxContentLength = DefaultMaxContent
		}
		if d.MaxContentLength == nil {
			d.MaxContentLength = new(int64)
			if d.ReadMode != Stream {
				*d.MaxContentLength = DefaultMaxContent
			}
		}
		if d.MinContentLength == nil {
			d.MinContentLength = new(int)
			*d.MinContentLength = 0
		}
		if d.Timeout == time.Duration(0) {
			d.Timeout = DatasourceDefaultTimeout
		}
		if d.Timeout < time.Millisecond {
			return fmt.Errorf("timeout value cannot be less than 1 ms: timeout=%s", d.Timeout)
		}
		if len(d.LineSeparator) == 0 {
			d.LineSeparator = []string{"\n"}
		}
	}
	d.HTTPConfig = nil
	d.TCPConfig = nil
	d.UDPConfig = nil
	return nil
}

func (d *Datasource) ReadAll(ctx context.Context) ([]byte, error) {
	var reader io.Reader
	rc, err := d.GetStream(ctx)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	reader = io.LimitReader(rc, *d.MaxContentLength)
	if d.MinContentLength != nil && *d.MinContentLength > 0 {
		data, err := io.ReadAll(reader)
		if err != nil {
			return nil, err
		}
		if len(data) < *d.MinContentLength {
			return nil, ErrorDataTooSort
		}
		return data, nil
	} else {
		return io.ReadAll(reader)
	}
}

func (d *Datasource) GetLineStream(ctx context.Context, logger log.Logger) (buffer.ReadLineCloser, error) {
	if d.MaxContentLength == nil {
		d.MaxContentLength = new(int64)
		*d.MaxContentLength = DefaultMaxContent
	}
	if d.LineMaxContentLength == nil {
		d.LineMaxContentLength = new(int)
		*d.LineMaxContentLength = DefaultMaxContent
	}

	if d.Type.ToLower() == File && d.ReadMode.ToLower() == Stream {
		if t, err := tail.TailFile(d.Url, tail.Config{
			Location:    &tail.SeekInfo{Whence: d.Whence},
			Follow:      true,
			ReOpen:      true,
			LineSep:     d.LineSeparator,
			MaxLineSize: *d.LineMaxContentLength,
			Logger:      stdlog.New(log.NewStdlibAdapter(logger), "", 0),
		}); err != nil {
			return nil, fmt.Errorf("Failed to open file %s: %s. ", d.Url, err)
		} else {
			return t, nil
		}
	}
	rc, err := d.GetStream(ctx)
	if err != nil {
		return nil, err
	}

	return buffer.NewLineBuffer(rc, *d.MaxContentLength, *d.LineMaxContentLength, d.LineSeparator, []byte(d.EndOf)), nil
}

func (d *Datasource) GetStream(ctx context.Context) (io.ReadCloser, error) {
	switch d.Type.ToLower() {
	case Http, Tcp, Udp:
		if body, err := d.Config.GetStream(ctx, d.Name, d.Url); err != nil {
			return nil, fmt.Errorf("Request URL %s failed: %s. ", d.Url, err)
		} else {
			return body, nil
		}
	case File:
		if file, err := os.Open(d.Url); err != nil {
			return nil, fmt.Errorf("Failed to open file %s: %s. ", d.Url, err)
		} else {
			return file, nil
		}
	default:
		return nil, fmt.Errorf("unknown datasource type: %s", d.Type)
	}
}
func (d *Datasource) Close() {
	if d.r != nil {
		d.r.Close()
	}
}
