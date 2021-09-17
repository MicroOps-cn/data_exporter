package collector

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
)

type DatasourceType string

const (
	Http DatasourceType = "http"
	File DatasourceType = "file"
)

func (d DatasourceType) ToLower() DatasourceType {
	return DatasourceType(strings.ToLower(string(d)))
}

type DatasourceTypeReadMode string

const (
	StreamLine DatasourceTypeReadMode = "stream-line"
	FullText   DatasourceTypeReadMode = "full-text"
	Line       DatasourceTypeReadMode = "line"
)

func (d DatasourceTypeReadMode) ToLower() DatasourceTypeReadMode {
	return DatasourceTypeReadMode(strings.ToLower(string(d)))
}

type Datasource struct {
	Name             string                 `yaml:"name"`
	MaxContentLength int64                  `yaml:"max_content_length"`
	Url              string                 `yaml:"url"`
	Type             DatasourceType         `yaml:"type"`
	RelabelConfigs   RelabelConfigs         `yaml:"relabel_configs"`
	ReadMode         DatasourceTypeReadMode `yaml:"read_mode"`
}

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
		if d.ReadMode == "stream" {
			d.ReadMode = StreamLine
		}
	}
	return nil
}

type DatasourceReaderCloser interface {
	io.ReadCloser
	io.Closer
	SeekStart()
}

func (d *Datasource) getData() ([]byte, error) {
	var reader io.Reader
	switch d.Type.ToLower() {
	case Http:
		if resp, err := http.Get(d.Url); err != nil {
			return nil, fmt.Errorf("Request URL %s failed: %s ", d.Url, err)
		} else {
			defer resp.Body.Close()
			reader = resp.Body
		}
	case File:
		if file, err := os.Open(d.Url); err != nil {
			return nil, fmt.Errorf("Failed to open file %s: %s ", d.Url, err)
		} else {
			defer file.Close()
			reader = file
		}
	default:
		return nil, fmt.Errorf("unknown datasource type: %s", d.Type)
	}
	reader = io.LimitReader(reader, d.MaxContentLength)
	if data, err := ioutil.ReadAll(reader); err != nil {
		return nil, err
	} else {
		return data, nil
	}
}

func (d *Datasource) getStream() (io.ReadCloser, error) {
	switch d.Type.ToLower() {
	case Http:
		if resp, err := http.Get(d.Url); err != nil {
			return nil, fmt.Errorf("Request URL %s failed: %s ", d.Url, err)
		} else {
			return resp.Body, nil
		}
	case File:
		if file, err := os.Open(d.Url); err != nil {
			return nil, fmt.Errorf("Failed to open file %s: %s ", d.Url, err)
		} else {
			return file, nil
		}
	default:
		return nil, fmt.Errorf("unknown datasource type: %s", d.Type)
	}
}
