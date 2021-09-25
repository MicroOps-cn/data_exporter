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
	"context"
	"fmt"
	"github.com/prometheus/common/config"
	"gopkg.in/alecthomas/kingpin.v2"
	"gopkg.in/yaml.v3"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"time"
)

type DatasourceType string

const (
	Http DatasourceType = "http"
	File DatasourceType = "file"
	Tcp  DatasourceType = "tcp"
	Udp  DatasourceType = "udp"
)

func (d DatasourceType) ToLower() DatasourceType {
	return DatasourceType(strings.ToLower(string(d)))
}

type DatasourceReadMode string

const (
	StreamLine DatasourceReadMode = "stream-line"
	FullText   DatasourceReadMode = "full-text"
)

func (d DatasourceReadMode) ToLower() DatasourceReadMode {
	return DatasourceReadMode(strings.ToLower(string(d)))
}

type HTTPConfig struct {
	HTTPClientConfig config.HTTPClientConfig `yaml:"http_client_config,inline"`
	Body             string                  `yaml:"body,omitempty"`
	Headers          map[string]string       `yaml:"headers,omitempty"`
	Method           string                  `yaml:"method,omitempty"`
	ValidStatusCodes []int                   `yaml:"valid_status_codes,omitempty"`
}

func (h HTTPConfig) GetStream(ctx context.Context, name, targetURL string) (io.ReadCloser, error) {
	client, err := config.NewClientFromConfig(h.HTTPClientConfig, name, config.WithKeepAlivesDisabled())
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
		if strings.Title(key) == "Host" {
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

type SendConfig struct {
	Msg   string        `yaml:"msg,omitempty"`
	Delay time.Duration `yaml:"timeout,omitempty"`
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

type Datasource struct {
	Name             string             `yaml:"name"`
	Url              string             `yaml:"url"`
	Type             DatasourceType     `yaml:"type"`
	Timeout          time.Duration      `yaml:"timeout"`
	ReadMode         DatasourceReadMode `yaml:"read_mode"`
	HTTPConfig       *HTTPConfig        `yaml:"http"`
	TCPConfig        *NetConfig         `yaml:"tcp"`
	UDPConfig        *NetConfig         `yaml:"udp"`
	RelabelConfigs   RelabelConfigs     `yaml:"relabel_configs"`
	MaxContentLength int64              `yaml:"max_content_length"`
}

var (
	DefaultHttpConfig = HTTPConfig{Method: "GET"}
	DefaultTimeout    = kingpin.Flag("timeout", "Default timeout").Default("30s").Duration()
)

func (d *Datasource) UnmarshalYAML(value *yaml.Node) error {
	type plain Datasource
	if err := value.Decode((*plain)(d)); err != nil {
		return err
	} else {
		if d.MaxContentLength <= 0 {
			d.MaxContentLength = 102400000
		}
		d.ReadMode = d.ReadMode.ToLower()
		if d.ReadMode == "" {
			d.ReadMode = FullText
		}
		if d.ReadMode == "stream" || d.ReadMode == "line" {
			d.ReadMode = StreamLine
		}
		switch d.Type {
		case Http:
			if d.HTTPConfig == nil {
				d.HTTPConfig = &DefaultHttpConfig
			}
		}
		if d.Timeout == 0 {
			d.Timeout = *DefaultTimeout
		}
		if d.Timeout < time.Millisecond {
			return fmt.Errorf("timeout value cannot be less than 1 ms")
		}
	}
	return nil
}

func (d *Datasource) ReadAll(ctx context.Context) ([]byte, error) {
	var reader io.Reader
	rc, err := d.getStream(ctx)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	reader = io.LimitReader(rc, d.MaxContentLength)
	return ioutil.ReadAll(reader)
}

func (d *Datasource) GetLineStream(ctx context.Context) (ReadLineCloser, error) {
	rc, err := d.getStream(ctx)
	if err != nil {
		return nil, err
	}
	return &ReadLineClose{
		Closer: rc,
		buf:    bufio.NewScanner(io.LimitReader(rc, d.MaxContentLength)),
	}, nil
}

func (d *Datasource) getStream(ctx context.Context) (io.ReadCloser, error) {
	switch d.Type.ToLower() {
	case Http:
		if body, err := d.HTTPConfig.GetStream(ctx, d.Name, d.Url); err != nil {
			return nil, fmt.Errorf("Request URL %s failed: %s. ", d.Url, err)
		} else {
			return body, nil
		}
	case Tcp:
		if body, err := d.TCPConfig.GetStream(ctx, d.Name, d.Url, string(d.Type)); err != nil {
			return nil, fmt.Errorf("Request URL %s failed: %s. ", d.Url, err)
		} else {
			return body, nil
		}
	case Udp:
		if body, err := d.UDPConfig.GetStream(ctx, d.Name, d.Url, string(d.Type)); err != nil {
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

type ReadLineCloser interface {
	io.Closer
	ReadLine() ([]byte, error)
}
type ReadLineClose struct {
	io.Closer
	buf *bufio.Scanner
}

func (r *ReadLineClose) ReadLine() ([]byte, error) {
	if r.buf.Scan() {
		return r.buf.Bytes(), nil
	} else {
		return nil, io.EOF
	}
}
