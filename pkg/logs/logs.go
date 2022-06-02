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
	fmt.Println(config.Format.String())
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
		logFormat.Set(config.Format.String())
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
