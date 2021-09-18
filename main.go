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
	"errors"
	"fmt"
	"gitee.com/paycooplus/data_exporter/collector"
	"gitee.com/paycooplus/data_exporter/config"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	promcollectors "github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/promlog"
	"github.com/prometheus/common/promlog/flag"
	"github.com/prometheus/common/version"
	"github.com/prometheus/exporter-toolkit/web"
	webflag "github.com/prometheus/exporter-toolkit/web/kingpinflag"
	"gopkg.in/alecthomas/kingpin.v2"
	stdlog "log"
	"net"
	"net/http"
	"net/http/pprof"
	"net/url"
	"os"
	"os/signal"
	"path"
	"strings"
	"syscall"
)

var (
	sc            = config.NewConfig()
	exporterName  = config.ExporterName
	webConfig     = webflag.AddFlags(kingpin.CommandLine)
	listenAddress = kingpin.Flag("web.listen-address", "The address to listen on for HTTP requests.").Default(":9116").String()
	configFile    = kingpin.Flag("config.file", "Blackbox exporter configuration file.").Default(exporterName + ".yaml").String()
	routePrefix   = kingpin.Flag("web.route-prefix", "Prefix for the internal routes of web endpoints. Defaults to path of --web.external-url.").PlaceHolder("<path>").String()
	configCheck   = kingpin.Flag("config.check", "If true validate the config file and then exit.").Default().Bool()
	externalURL   = kingpin.Flag("web.external-url", "The URL under which Blackbox exporter is externally reachable (for example, if Blackbox exporter is served via a reverse proxy). Used for generating relative and absolute links back to Blackbox exporter itself. If the URL has a path portion, it will be used to prefix all HTTP endpoints served by Blackbox exporter. If omitted, relevant URL components will be derived automatically.").PlaceHolder("<url>").String()
	enablePprof   = kingpin.Flag("pprof.enable", "Enable pprof").Bool()
	pprofUrl      = kingpin.Flag("pprof.url", "pprof url prefix").Default("/-/pprof/").String()
)

func main() {
	os.Exit(run())
}

func run() int {
	promlogConfig := &promlog.Config{}
	flag.AddFlags(kingpin.CommandLine, promlogConfig)
	kingpin.Version(version.Print(exporterName))
	kingpin.HelpFlag.Short('h')
	kingpin.Parse()
	logger := promlog.New(promlogConfig)
	level.Info(logger).Log("msg", "Starting "+exporterName, "version", version.Info())
	level.Info(logger).Log("build_context", version.BuildContext())
	if err := sc.ReloadConfig(*configFile, logger); err != nil {
		level.Error(logger).Log("msg", "Error loading config", "err", err)
		return 1
	}

	if *configCheck {
		level.Info(logger).Log("msg", "Config file is ok exiting...")
		return 0
	}
	level.Info(logger).Log("msg", "Loaded config file")
	beURL, err := computeExternalURL(*externalURL, *listenAddress)
	if err != nil {
		level.Error(logger).Log("msg", "failed to determine external URL", "err", err)
		return 1
	}
	level.Debug(logger).Log("externalURL", beURL.String())

	// Default -web.route-prefix to path of -web.external-url.
	if *routePrefix == "" {
		*routePrefix = beURL.Path
	}

	// routePrefix must always be at least '/'.
	*routePrefix = "/" + strings.Trim(*routePrefix, "/")
	// routePrefix requires path to have trailing "/" in order
	// for browsers to interpret the path-relative path correctly, instead of stripping it.
	if *routePrefix != "/" {
		*routePrefix = *routePrefix + "/"
	}
	level.Debug(logger).Log("routePrefix", *routePrefix)

	hup := make(chan os.Signal, 1)
	reloadCh := make(chan chan error)
	signal.Notify(hup, syscall.SIGHUP)
	go func() {
		for {
			select {
			case <-hup:
				if err := sc.ReloadConfig(*configFile, logger); err != nil {
					level.Error(logger).Log("msg", "Error reloading config", "err", err)
					continue
				}
				level.Info(logger).Log("msg", "Reloaded config file")
			case rc := <-reloadCh:
				if err := sc.ReloadConfig(*configFile, logger); err != nil {
					level.Error(logger).Log("msg", "Error reloading config", "err", err)
					rc <- err
				} else {
					level.Info(logger).Log("msg", "Reloaded config file")
					rc <- nil
				}
			}
		}
	}()

	serve := http.NewServeMux()
	// Match Prometheus behaviour and redirect over externalURL for root path only
	// if routePrefix is different than "/"
	serve.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	serve.HandleFunc(path.Join(*routePrefix, "/-/reload"),
		func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "POST" {
				w.WriteHeader(http.StatusMethodNotAllowed)
				fmt.Fprintf(w, "This endpoint requires a POST request.\n")
				return
			}

			rc := make(chan error)
			reloadCh <- rc
			if err := <-rc; err != nil {
				http.Error(w, fmt.Sprintf("failed to reload config: %s", err), http.StatusInternalServerError)
			}
		})

	serve.HandleFunc(path.Join(*routePrefix, "/metrics"), func(writer http.ResponseWriter, request *http.Request) {
		collectMetrics(logger, writer, request)
	})

	serve.HandleFunc(path.Join(*routePrefix, "/-/healthy"), func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Healthy"))
	})
	if *enablePprof {
		pprofPrefix := path.Join(*routePrefix, *pprofUrl) + "/"
		serve.HandleFunc(pprofPrefix, func(w http.ResponseWriter, r *http.Request) {
			name := strings.TrimPrefix(r.URL.Path, pprofPrefix)
			if name == "" {
				r.URL.Path += "/"
			} else if name != "/" {
				r.URL.Path = path.Join("/debug/pprof/", name)
			}
			pprof.Index(w, r)
		})
		serve.HandleFunc(path.Join(*routePrefix, *pprofUrl, "cmdline"), pprof.Cmdline)
		serve.HandleFunc(path.Join(*routePrefix, *pprofUrl, "profile"), pprof.Profile)
		serve.HandleFunc(path.Join(*routePrefix, *pprofUrl, "symbol"), pprof.Symbol)
		serve.HandleFunc(path.Join(*routePrefix, *pprofUrl, "trace"), pprof.Trace)
	}
	srv := &http.Server{Addr: *listenAddress, Handler: serve}
	srvc := make(chan struct{})
	term := make(chan os.Signal, 1)
	signal.Notify(term, os.Interrupt, syscall.SIGTERM)
	go func() {
		level.Info(logger).Log("msg", "Listening on address", "address", *listenAddress)
		if err := web.ListenAndServe(srv, *webConfig, logger); err != http.ErrServerClosed {
			level.Error(logger).Log("msg", "Error starting HTTP server", "err", err)
			close(srvc)
		}
	}()

	for {
		select {
		case <-term:
			level.Info(logger).Log("msg", "Received SIGTERM, exiting gracefully...")
			return 0
		case <-srvc:
			return 1
		}
	}
}

func startsOrEndsWithQuote(s string) bool {
	return strings.HasPrefix(s, "\"") || strings.HasPrefix(s, "'") ||
		strings.HasSuffix(s, "\"") || strings.HasSuffix(s, "'")
}

// computeExternalURL computes a sanitized external URL from a raw input. It infers unset
// URL parts from the OS and the given listen address.
func computeExternalURL(u, listenAddr string) (*url.URL, error) {
	if u == "" {
		hostname, err := os.Hostname()
		if err != nil {
			return nil, err
		}
		_, port, err := net.SplitHostPort(listenAddr)
		if err != nil {
			return nil, err
		}
		u = fmt.Sprintf("http://%s:%s/", hostname, port)
	}

	if startsOrEndsWithQuote(u) {
		return nil, errors.New("URL must not begin or end with quotes")
	}

	eu, err := url.Parse(u)
	if err != nil {
		return nil, err
	}

	ppref := strings.TrimRight(eu.Path, "/")
	if ppref != "" && !strings.HasPrefix(ppref, "/") {
		ppref = "/" + ppref
	}
	eu.Path = ppref

	return eu, nil
}

func collectMetrics(logger log.Logger, w http.ResponseWriter, r *http.Request) {
	sc.Lock()
	conf := sc.C
	sc.Unlock()
	reg := prometheus.NewRegistry()
	for idx := range conf.Collects {
		conf.Collects[idx].SetLogger(logger)
		reg.MustRegister(&conf.Collects[idx])
	}
	reg.MustRegister(
		promcollectors.NewProcessCollector(promcollectors.ProcessCollectorOpts{}),
		promcollectors.NewGoCollector(),
		version.NewCollector(exporterName),
	)
	config.RegisterCollector(reg)
	collector.RegisterCollector(reg)
	handler := promhttp.HandlerFor(
		prometheus.Gatherers{reg},
		promhttp.HandlerOpts{
			ErrorLog:            stdlog.New(log.NewStdlibAdapter(level.Error(logger)), "", 0),
			ErrorHandling:       promhttp.ContinueOnError,
			MaxRequestsInFlight: 10,
			Registry:            reg,
		},
	)
	handler.ServeHTTP(w, r)
}
