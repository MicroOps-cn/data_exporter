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

package logs

import (
	"fmt"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/pkg/errors"
	"github.com/prometheus/common/promlog"
	"github.com/prometheus/common/promlog/flag"
	"gopkg.in/alecthomas/kingpin.v2"
	"os"
)

// AllowedFormat is a settable identifier for the output format that the logger can have.
type AllowedFormat struct {
	s string
}

func (f *AllowedFormat) String() string {
	return f.s
}

// Set updates the value of the allowed format.
func (f *AllowedFormat) Set(s string) error {
	switch s {
	case "logfmt", "json", "debug":
		f.s = s
	default:
		return errors.Errorf("unrecognized log format %q", s)
	}
	return nil
}

type Config struct {
	Level  *promlog.AllowedLevel
	Format *AllowedFormat
}

func New(config *Config) log.Logger {
	if config.Level == nil {
		config.Level = &promlog.AllowedLevel{}
	}
	if config.Level.String() == "" {
		err := config.Level.Set("info")
		if err != nil {
			panic(err)
		}
	}
	if config.Format == nil {
		config.Format = &AllowedFormat{}
	}
	if config.Format.String() == "" {
		err := config.Format.Set("logfmt")
		if err != nil {
			panic(err)
		}
	}
	switch config.Format.String() {
	case "debug":
		var l log.Logger = newDebugLogger(log.NewSyncWriter(os.Stderr))
		if config.Level != nil {
			var lvl level.Option
			switch config.Level.String() {
			case "debug":
				lvl = level.AllowDebug()
			case "info":
				lvl = level.AllowInfo()
			case "warn":
				lvl = level.AllowWarn()
			case "error":
				lvl = level.AllowError()
			default:
				panic(fmt.Sprintf("unrecognized log level %q", config.Level.String()))
			}
			l = level.NewFilter(l, lvl)
		}
		return l
	default:
		logFormat := promlog.AllowedFormat{}
		if err := logFormat.Set(config.Format.String()); err != nil {
			panic(err)
		}
		return promlog.New(&promlog.Config{Level: config.Level, Format: &logFormat})
	}
}

func AddFlags(a *kingpin.Application, config *Config) {
	config.Level = &promlog.AllowedLevel{}
	a.Flag(flag.LevelFlagName, flag.LevelFlagHelp).
		Default("info").SetValue(config.Level)

	config.Format = &AllowedFormat{}
	a.Flag(flag.FormatFlagName, flag.FormatFlagHelp).
		Default("logfmt").SetValue(config.Format)
}
