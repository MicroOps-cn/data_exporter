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

package main

import (
	"fmt"
	"github.com/MicroOps-cn/data_exporter/collector"
	"github.com/MicroOps-cn/data_exporter/config"
	"github.com/MicroOps-cn/data_exporter/pkg/logs"
	"github.com/MicroOps-cn/data_exporter/pkg/term"
	"github.com/MicroOps-cn/data_exporter/pkg/wrapper"
	"github.com/MicroOps-cn/data_exporter/transport"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/promlog"
	"github.com/prometheus/common/version"
	"gopkg.in/alecthomas/kingpin.v2"
	"gopkg.in/yaml.v3"
	"io/ioutil"
	stdlog "log"
	"net/http"
	"net/http/httptest"
	"os"
	"os/signal"
	"syscall"
	"time"
)

var (
	sc            = config.NewSafeConfig()
	flagSet       = kingpin.New(os.Args[0], "Prometheus Common Data Exporter is used to parse JSON, XML, yaml or other format data from multiple sources (such as HTTP response message, local file, TCP response message and UDP response message) into Prometheus metric data.")
	exporterName  = collector.ExporterName
	debugFlagSet  = flagSet.Command("debug", "Debug configuration")
	runFlagSet    = flagSet.Command("run", "Run a exporter")
	verifyFlagSet = flagSet.Command("verify", "verify configuration")

	displayConfig = verifyFlagSet.Flag("config.display", "display configuration").Bool()

	// Deprecated
	configFile = flagSet.Flag("config.file", "[Deprecated]Blackbox exporter configuration file.").String()
	configPath = flagSet.Flag("config.path", "Blackbox exporter configuration path. can be a directory").Default(exporterName + ".yaml").String()

	promlogConfig = &logs.Config{}
	rootLogger    log.Logger
)

func main() {
	kingpin.MustParse(flagSet.Parse(os.Args[1:]))
}

func init() {
	logs.AddFlags(flagSet, promlogConfig)
	flagSet.Version(version.Print(exporterName))
	flagSet.HelpFlag.Short('h')
	collector.AddFlags(flagSet)
	transport.AddFlags(flagSet, runFlagSet)

	flagSet.PreAction(func(pCtx *kingpin.ParseContext) error {
		for _, element := range pCtx.Elements {
			if flag, ok := element.Clause.(*kingpin.FlagClause); ok && (flag == flagSet.HelpFlag || flag == flagSet.VersionFlag) {
				return nil
			}
		}
		rootLogger = logs.New(promlogConfig)
		if len(*configFile) > 0 {
			*configPath = *configFile
		}
		if err := sc.ReloadConfig(*configPath, rootLogger); err != nil {
			level.Error(rootLogger).Log("msg", "Error loading config", "err", err, "configPath", configPath)
			return err
		}
		level.Info(rootLogger).Log("msg", "Loaded config file")
		return nil
	})

	runFlagSet.Action(func(_ *kingpin.ParseContext) error {
		return run(rootLogger)
	}).Default()

	debugFlagSet.Action(func(_ *kingpin.ParseContext) error {
		debugLogCfg := logs.Config{Level: &promlog.AllowedLevel{}, Format: &logs.AllowedFormat{}}
		if err := debugLogCfg.Level.Set("debug"); err != nil {
			return err
		}
		if err := debugLogCfg.Format.Set("debug"); err != nil {
			return err
		}
		debugLogger := logs.New(&debugLogCfg)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			conf := sc.GetConfig()
			time.Sleep(time.Second / 2)
			reg := prometheus.NewRegistry()
			for idx := range conf.Collects {
				conf.Collects[idx].SetLogger(debugLogger)
				reg.MustRegister(&collector.CollectContext{
					CollectConfig: &conf.Collects[idx],
					Context:       r.Context(),
				})
			}
			reg2 := prometheus.NewRegistry()
			collector.RegisterCollector(reg2)
			handler := promhttp.HandlerFor(
				prometheus.Gatherers{reg, reg2},
				promhttp.HandlerOpts{
					ErrorLog:            stdlog.New(log.NewStdlibAdapter(level.Error(rootLogger)), "", 0),
					ErrorHandling:       promhttp.ContinueOnError,
					MaxRequestsInFlight: 10,
				},
			)
			handler.ServeHTTP(w, r)
		}))
		term.Title("Process", '=')
		resp := wrapper.M[*http.Response](http.Get(server.URL))
		if resp.StatusCode != 200 {
			panic(fmt.Sprintf("Unknown error: %d!=200", resp.StatusCode))
		}
		metrics := wrapper.M[[]byte](ioutil.ReadAll(resp.Body))
		term.Title("Output Metrics", '=')
		fmt.Println(string(metrics))
		return nil
	})

	verifyFlagSet.Action(func(_ *kingpin.ParseContext) error {
		level.Info(rootLogger).Log("msg", "Config file is ok, exiting...")
		if *displayConfig {
			_ = yaml.NewEncoder(os.Stdout).Encode(sc.GetConfig())
		}
		return nil
	})

}

func run(logger log.Logger) error {
	level.Info(logger).Log("msg", "Starting "+exporterName, "version", version.Info())
	level.Info(logger).Log("build_context", version.BuildContext())

	hup := make(chan os.Signal, 1)
	reloadCh := make(chan chan error)
	signal.Notify(hup, syscall.SIGHUP)
	go func() {
		for {
			select {
			case <-hup:
				level.Info(logger).Log("msg", "Reload config from signal")
				if err := sc.ReloadConfig(*configPath, logger); err != nil {
					level.Error(logger).Log("msg", "Error reloading config", "err", err)
					continue
				}
				level.Info(logger).Log("msg", "Reloaded config file")
			case rc := <-reloadCh:
				if err := sc.ReloadConfig(*configPath, logger); err != nil {
					level.Error(logger).Log("msg", "Error reloading config", "err", err)
					rc <- err
				} else {
					level.Info(logger).Log("msg", "Reloaded config file")
					rc <- nil
				}
			}
		}
	}()

	serve, err := transport.NewHttpServer(logger, sc)

	serve.HandleFunc("/-/reload",
		func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "POST" {
				w.WriteHeader(http.StatusMethodNotAllowed)
				_, _ = fmt.Fprintf(w, "This endpoint requires a POST request.\n")
				return
			}
			level.Info(logger).Log("msg", "Reload config from http api")
			rc := make(chan error)
			reloadCh <- rc
			if err := <-rc; err != nil {
				http.Error(w, fmt.Sprintf("failed to reload config: %s", err), http.StatusInternalServerError)
			}
		})

	srvc := make(chan error)
	termSigCh := make(chan os.Signal, 1)
	signal.Notify(termSigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		if err = serve.ListenAndServe(logger); err != http.ErrServerClosed {
			level.Error(logger).Log("msg", "Error starting HTTP server", "err", err)
			close(srvc)
		}
	}()

	for {
		select {
		case <-termSigCh:
			level.Info(logger).Log("msg", "Received SIGTERM, exiting gracefully...")
			return nil
		case err = <-srvc:
			return err
		}
	}
}
