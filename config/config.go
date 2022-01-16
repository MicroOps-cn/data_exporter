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

package config

import (
	"context"
	"fmt"
	"github.com/MicroOps-cn/data_exporter/collector"
	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
	"gopkg.in/yaml.v3"
	"io"
	"os"
	"sync"
)

const ExporterName string = "data_exporter"

var (
	configReloadSuccess = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: ExporterName,
		Name:      "config_last_reload_successful",
		Help:      "Blackbox exporter config loaded successfully.",
	})

	configReloadSeconds = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: ExporterName,
		Name:      "config_last_reload_success_timestamp_seconds",
		Help:      "Timestamp of the last successful configuration reload.",
	})
)

func RegisterCollector(reg prometheus.Registerer) {
	reg.MustRegister(configReloadSuccess, configReloadSeconds)
}

type Config struct {
	Collects   collector.Collects `yaml:"collects"`
	cancelFunc context.CancelFunc
	ctx        context.Context
}

func (c *Config) Init(logger log.Logger) error {
	c.Collects.SetLogger(logger)
	return c.Collects.StartStreamCollect(c.ctx)
}

func (c *Config) UnmarshalYAML(value *yaml.Node) error {
	type plain Config
	if err := value.Decode((*plain)(c)); err != nil {
		return err
	}
	c.ctx, c.cancelFunc = context.WithCancel(context.Background())
	return nil
}

type SafeConfig struct {
	sync.RWMutex
	C *Config
}

func NewConfig() *SafeConfig {
	return &SafeConfig{
		C: &Config{},
	}
}

func (sc *SafeConfig) ReloadConfigFromReader(reader io.Reader, logger log.Logger) (err error) {
	var c = &Config{}
	decoder := yaml.NewDecoder(reader)
	decoder.KnownFields(true)

	if err = decoder.Decode(c); err != nil {
		return fmt.Errorf("error parsing config file: %s", err)
	}

	if err = c.Init(logger); err != nil {
		return fmt.Errorf("error init config: %s", err)
	}
	if sc.C != nil {
		sc.C.Collects.StopStreamCollect()
		if sc.C.cancelFunc != nil {
			sc.C.cancelFunc()
		}
	}
	sc.Lock()
	sc.C = c
	sc.Unlock()
	return nil
}

func (sc *SafeConfig) SetConfig(conf *Config) {
	sc.Lock()
	defer sc.Unlock()
	sc.C = conf
}
func (sc *SafeConfig) GetConfig() *Config {
	sc.Lock()
	defer sc.Unlock()
	return sc.C
}
func (sc *SafeConfig) ReloadConfig(confFile string, logger log.Logger) (err error) {
	defer func() {
		if err != nil {
			configReloadSuccess.Set(0)
		} else {
			configReloadSuccess.Set(1)
			configReloadSeconds.SetToCurrentTime()
		}
	}()

	yamlReader, err := os.Open(confFile)
	if err != nil {
		return fmt.Errorf("error reading config file: %s", err)
	}
	defer yamlReader.Close()
	return sc.ReloadConfigFromReader(yamlReader, logger)
}
