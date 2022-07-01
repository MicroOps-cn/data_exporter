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
	"path"
	"sync"
)

var (
	configReloadSuccess = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: collector.ExporterName,
		Name:      "config_last_reload_successful",
		Help:      "Blackbox exporter config loaded successfully.",
	})

	configReloadSeconds = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: collector.ExporterName,
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
	return nil
}

type SafeConfig struct {
	sync.RWMutex
	C *Config
}

func NewSafeConfig() *SafeConfig {
	return &SafeConfig{
		C: NewConfig(),
	}
}

func NewConfig() *Config {
	ctx, cancelFunc := context.WithCancel(context.Background())
	return &Config{ctx: ctx, cancelFunc: cancelFunc}
}

func (sc *SafeConfig) ReloadConfigFromReader(reader io.ReadCloser, logger log.Logger) (err error) {
	var c = NewConfig()
	if err = c.loadConfigFile(reader); err != nil {
		return err
	} else if err = c.Init(logger); err != nil {
		return err
	}
	sc.Lock()
	defer sc.Unlock()
	sc.C = c
	return
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
func (sc *SafeConfig) ReloadConfig(confPath string, logger log.Logger) (err error) {
	defer func() {
		if err != nil {
			configReloadSuccess.Set(0)
		} else {
			configReloadSuccess.Set(1)
			configReloadSeconds.SetToCurrentTime()
		}
	}()
	var c = NewConfig()
	if err = c.LoadConfig(confPath); err != nil {
		return err
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
	defer sc.Unlock()
	sc.C = c
	return nil
}

func (c *Config) loadConfigFile(cfgFile io.ReadCloser) error {
	defer cfgFile.Close()
	var tmpCfg Config
	decoder := yaml.NewDecoder(cfgFile)
	decoder.KnownFields(true)

	if err := decoder.Decode(&tmpCfg); err != nil {
		return fmt.Errorf("error parsing config file: %s", err)
	}
	c.Collects = append(c.Collects, tmpCfg.Collects...)
	return nil
}
func (c *Config) LoadConfig(configPath string) error {
	if stat, err := os.Stat(configPath); err != nil {
		return err
	} else {
		if stat.IsDir() {
			cfgPool := os.DirFS(configPath)
			if entrys, err := os.ReadDir(configPath); err != nil {
				return err
			} else {
				for _, entry := range entrys {
					if !entry.IsDir() {
						switch path.Ext(entry.Name()) {
						case ".yml", ".yaml":
							if f, err := cfgPool.Open(entry.Name()); err != nil {
								return err
							} else if err = c.loadConfigFile(f); err != nil {
								return err
							}
						}

					}
				}
			}
		} else {
			if f, err := os.Open(configPath); err != nil {
				return err
			} else {
				return c.loadConfigFile(f)
			}
		}
	}
	return nil
}
