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

package transport

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/MicroOps-cn/data_exporter/collector"
	"github.com/MicroOps-cn/data_exporter/config"
	"github.com/MicroOps-cn/data_exporter/pkg/wrapper"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	promcollectors "github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/version"
	"github.com/prometheus/exporter-toolkit/web"
	webflag "github.com/prometheus/exporter-toolkit/web/kingpinflag"
	"gopkg.in/alecthomas/kingpin.v2"
	"gopkg.in/yaml.v3"
	"io"
	"io/fs"
	stdlog "log"
	"net"
	"net/http"
	"net/http/pprof"
	"net/url"
	"os"
	"path"
	"strings"
	"time"
)

var (
	listenAddress *string = wrapper.P[string](":9116")
	routePrefix   *string = wrapper.P[string]("")
	enablePprof   *bool   = wrapper.P[bool](false)
	pprofUrl      *string = wrapper.P[string]("/-/pprof/")
	externalURL   *string = wrapper.P[string]("")
	enableUi      *bool   = wrapper.P[bool](false)
	uiPrefix      *string = wrapper.P[string]("/-/ui/")
	webConfig     *string = wrapper.P[string]("")

	//go:embed static
	staticFs embed.FS
)

func AddFlags(app *kingpin.Application, runFlagSet *kingpin.CmdClause) {
	enablePprof = runFlagSet.Flag("pprof.enable", "Enable pprof").Bool()
	pprofUrl = runFlagSet.Flag("pprof.url", "pprof url prefix").Default("/-/pprof/").String()
	listenAddress = runFlagSet.Flag("web.listen-address", "The address to listen on for HTTP requests.").Default(":9116").String()
	routePrefix = runFlagSet.Flag("web.route-prefix", "Prefix for the internal routes of web endpoints. Defaults to path of --web.external-url.").PlaceHolder("<path>").String()
	externalURL = runFlagSet.Flag("web.external-url", "The URL under which Blackbox exporter is externally reachable (for example, if Blackbox exporter is served via a reverse proxy). Used for generating relative and absolute links back to Blackbox exporter itself. If the URL has a path portion, it will be used to prefix all HTTP endpoints served by Blackbox exporter. If omitted, relevant URL components will be derived automatically.").PlaceHolder("<url>").String()
	enableUi = runFlagSet.Flag("web.ui-enable", "Enable UI. This UI is mainly used to assist users in debugging configurations, and will not change the running configuration files.").Bool()
	uiPrefix = runFlagSet.Flag("web.ui-prefix", "UI url prefix").Default("/-/ui/").String()
	webConfig = webflag.AddFlags(app)
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

type HttpServer struct {
	*http.ServeMux
	routePrefix string
	pprofPrefix string
	uiPrefix    string
	safeConfig  *config.SafeConfig
	uiServer    http.Handler
	pprofServer http.Handler
}

func getSafeUrlPrefix(prefix string) (safeUrlPrefix string) {
	safeUrlPrefix = "/" + strings.Trim(prefix, "/")
	if safeUrlPrefix != "/" {
		return safeUrlPrefix + "/"
	}
	return safeUrlPrefix
}

func NewHttpServer(logger log.Logger, sc *config.SafeConfig) (*HttpServer, error) {
	serve := &HttpServer{
		ServeMux:    http.NewServeMux(),
		safeConfig:  sc,
		routePrefix: *routePrefix,
		uiPrefix:    getSafeUrlPrefix(*uiPrefix),
		pprofPrefix: getSafeUrlPrefix(*pprofUrl),
	}

	beURL, err := computeExternalURL(*externalURL, *listenAddress)
	if err != nil {
		level.Error(logger).Log("msg", "failed to determine external URL", "err", err)
		return nil, err
	}
	level.Debug(logger).Log("externalURL", beURL.String())

	// Default -web.route-prefix to path of -web.external-url.
	if serve.routePrefix == "" {
		serve.routePrefix = beURL.Path
	}

	serve.routePrefix = getSafeUrlPrefix(serve.routePrefix)

	level.Debug(logger).Log("routePrefix", serve.routePrefix)

	if *enablePprof && serve.pprofPrefix != "/" {
		pFuncs := map[string]http.HandlerFunc{"cmdline": pprof.Cmdline, "profile": pprof.Profile, "symbol": pprof.Symbol, "trace": pprof.Trace}
		level.Debug(logger).Log("msg", "enable pprof", "pprofPrefix", serve.pattern(serve.pprofPrefix))
		serve.pprofServer = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			name := strings.TrimPrefix(r.URL.Path, serve.pprofPrefix)
			if name == "" {
				r.URL.Path += "/"
			} else if name != "/" {
				r.URL.Path = path.Join("/debug/pprof/", name)
			}
			if pFunc, ok := pFuncs[name]; ok {
				pFunc(w, r)
				return
			}
			pprof.Index(w, r)
		})
	}
	serve.HandleFunc("/-/healthy", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Healthy"))
	})
	if *enableUi && serve.uiPrefix != "/" {
		level.Debug(logger).Log("msg", "enable ui", "uiPrefix", serve.pattern(serve.uiPrefix))
		uiFs := wrapper.M[fs.FS](fs.Sub(staticFs, "static"))
		serve.uiServer = http.StripPrefix(path.Join(serve.uiPrefix, "static"), http.FileServer(http.FS(uiFs)))
		serve.HandleFunc(path.Join(serve.pattern(serve.uiPrefix), "api/datapoints"), func(writer http.ResponseWriter, request *http.Request) {
			serve.getDatapoints(logger, writer, request)
		})
		serve.HandleFunc(path.Join(serve.pattern(serve.uiPrefix), "api/metrics"), func(writer http.ResponseWriter, request *http.Request) {
			serve.getMetrics(logger, writer, request)
		})
		serve.HandleFunc(path.Join(serve.pattern(serve.uiPrefix), "api/load/data"), func(writer http.ResponseWriter, request *http.Request) {
			serve.loadData(logger, writer, request)
		})
	}
	serve.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if serve.uiPrefix != "/" && strings.HasPrefix(r.URL.Path, serve.uiPrefix) {
			if *enableUi && serve.uiServer != nil {
				serve.uiServer.ServeHTTP(w, r)
				return
			}
		} else if serve.pprofPrefix != "/" && strings.HasPrefix(r.URL.Path, serve.pprofPrefix) {
			if *enablePprof && serve.pprofServer != nil {
				serve.pprofServer.ServeHTTP(w, r)
				return
			}
		} else {
			if strings.HasSuffix(r.URL.Path, "/metrics") {
				if r.URL.Path == "/metrics" {
					serve.collectMetrics(logger, w, r)
					return
				} else {
					serve.collectMetricsByName(logger, strings.TrimSuffix(strings.Trim(r.URL.Path, "/"), "/metrics"), w, r)
					return
				}
			}
		}
		http.NotFound(w, r)
	})
	return serve, nil
}

func (s *HttpServer) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	s.ServeMux.ServeHTTP(writer, request)
}

func (s *HttpServer) HandleFunc(pattern string, handler http.HandlerFunc) {
	if pattern == "/" {
		s.ServeMux.Handle("/", http.StripPrefix(strings.TrimRight(s.routePrefix, "/"), handler))
		return
	}
	s.ServeMux.Handle(path.Join(s.routePrefix, pattern), http.StripPrefix(strings.TrimRight(s.routePrefix, "/"), handler))
}

func (s *HttpServer) ListenAndServe(logger log.Logger) error {
	srv := &http.Server{Addr: *listenAddress, Handler: s}
	level.Info(logger).Log("msg", "Listening on address", "address", *listenAddress)
	return web.ListenAndServe(srv, *webConfig, logger)
}

func (s *HttpServer) pattern(pattern string) string {
	return path.Join(*routePrefix, pattern)
}

func (s *HttpServer) collectMetricsByName(logger log.Logger, name string, w http.ResponseWriter, r *http.Request) {
	level.Debug(logger).Log("msg", "collect metrics by collect_name", "name", name)
	conf := s.safeConfig.GetConfig()
	reg := prometheus.NewRegistry()
	if collect := conf.Collects.Get(name); collect != nil {
		collectCtx := &collector.CollectContext{
			CollectConfig: collect,
			Context:       r.Context(),
		}
		if r.URL.Query().Has("datasource") {
			collectCtx.DatasourceName = r.URL.Query().Get("datasource")
			if r.URL.Query().Has("url") {
				collectCtx.DatasourceUrl = r.URL.Query().Get("url")
			}
		}
		reg.MustRegister(collectCtx)
	} else {
		http.NotFound(w, r)
		return
	}

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

func (s *HttpServer) collectMetrics(logger log.Logger, w http.ResponseWriter, r *http.Request) {
	conf := s.safeConfig.GetConfig()
	reg := prometheus.NewRegistry()
	for idx := range conf.Collects {
		reg.MustRegister(&collector.CollectContext{
			CollectConfig: &conf.Collects[idx],
			Context:       r.Context(),
		})
	}
	reg.MustRegister(
		promcollectors.NewProcessCollector(promcollectors.ProcessCollectorOpts{}),
		promcollectors.NewGoCollector(),
		version.NewCollector(collector.ExporterName),
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

type GetDatapointsRequest struct {
	Data       string               `json:"data"`
	Rule       string               `json:"rule"`
	Mode       collector.DataFormat `json:"mode"`
	LabelMatch map[string]string    `json:"label_match"`
}

func (s *HttpServer) error(logger log.Logger, w http.ResponseWriter, err error) {
	level.Error(logger).Log("mes", "failed to decode request", "err", err)
	w.WriteHeader(500)
	_, _ = w.Write([]byte("failed to decode request."))
}

func (s *HttpServer) decodeRequest(logger log.Logger, req interface{}, w http.ResponseWriter, r *http.Request) error {
	err := json.NewDecoder(r.Body).Decode(req)
	defer r.Body.Close()
	if err != nil {
		s.error(logger, w, err)
	}
	return err
}

func (s *HttpServer) getDatapoints(logger log.Logger, w http.ResponseWriter, r *http.Request) {
	var req GetDatapointsRequest
	if err := s.decodeRequest(logger, &req, w, r); err != nil {
		return
	}

	var mc collector.MetricConfig
	mc.Match.Datapoint = req.Rule
	mc.Match.Labels = req.LabelMatch
	var dps []collector.Datapoint
	switch req.Mode {
	case collector.Regex:
		if err := mc.BuildRegexp(""); err != nil {
			s.error(logger, w, err)
			return
		}
		dps = mc.GetDatapointsByRegex(logger, []byte(req.Data))
	case collector.Xml:
		if err := mc.BuildTemplate(""); err != nil {
			s.error(logger, w, err)
			return
		}
		dps = mc.GetDatapointsByXml(logger, []byte(req.Data))
	case collector.Json:
		dps = mc.GetDatapointsByJson(logger, []byte(req.Data))
	case collector.Yaml:
		dps = mc.GetDatapointsByYaml(logger, []byte(req.Data))
	}
	_ = json.NewEncoder(w).Encode(dps)
}

type GetMetricsRequest struct {
	Datapoints     []collector.Datapoint    `json:"datapoints"`
	RelabelConfigs collector.RelabelConfigs `json:"relabel_configs"`
	MetricConfig   collector.MetricConfig   `json:"metric_config"`
}

func (s *HttpServer) getMetrics(logger log.Logger, w http.ResponseWriter, r *http.Request) {
	var req GetMetricsRequest
	if err := s.decodeRequest(logger, &req, w, r); err != nil {
		return
	}

	mgs := collector.NewMetricGenerators(10, logger)
	go func() {
		var err error
		for _, dp := range req.Datapoints {
			m := collector.NewMetricGenerator(logger, "", collector.Gauge)
			for name, val := range dp {
				m.Labels.Append(name, val)
			}
			m.Labels, err = req.MetricConfig.Relabels(logger, req.RelabelConfigs, m.Labels)
			if err != nil {
				continue
			}

			mgs.Ch() <- *m
		}
		close(mgs.Ch())
	}()
	reg := prometheus.NewRegistry()
	reg.MustRegister(mgs)
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

type LoadDataRequest struct {
	DataFormat collector.DataFormat    `yaml:"data_format"`
	Datasource []*collector.Datasource `yaml:"datasource"`
}

func (s *HttpServer) loadData(logger log.Logger, w http.ResponseWriter, r *http.Request) {
	var ds collector.Datasource

	err := yaml.NewDecoder(r.Body).Decode(&ds)
	defer r.Body.Close()
	if err != nil {
		s.error(logger, w, err)
		return
	}
	ctx, cancelFunc := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelFunc()
	var line []byte
	if ds.ReadMode == collector.Line {
		stream, err := ds.GetLineStream(ctx, logger)
		defer stream.Close()
		if err != nil {
			s.error(logger, w, err)
			return
		}
		line, err = stream.ReadLine()
		if err != nil {
			if io.EOF != err {
				s.error(logger, w, err)
				return
			}
			return
		}
	} else if ds.ReadMode == collector.Full {
		line, err = ds.ReadAll(ctx)
		if err != nil {
			s.error(logger, w, err)
			return
		}
	}
	_, _ = w.Write(line)
}
