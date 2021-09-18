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
	"fmt"
	"github.com/go-kit/log"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
)

var xmlContent = `
<root>
	<china dn="day">
		<weather>
			<city quName="黑龙江" pyName="heilongjiang" cityname="哈尔滨" state1="4" state2="1" stateDetailed="雷阵雨转多云" tem1="20"
				  tem2="9" windState="西南风3-4级转西风微风级">15</city>
			<city quName="吉林" pyName="jilin" cityname="长春" state1="7" state2="1" stateDetailed="小雨转多云" tem1="20" tem2="10"
				  windState="西风转东南风微风级">16</city>
			<city quName="辽宁" pyName="liaoning" cityname="沈阳" state1="3" state2="1" stateDetailed="阵雨转多云" tem1="21"
				  tem2="15" windState="北风3-4级">18</city>
			<city quName="海南" pyName="hainan" cityname="海口" state1="1" state2="1" stateDetailed="多云" tem1="32" tem2="26"
				  windState="微风">30</city>
		</weather>
	</china>
	<china dn="week">
			<city quName="黑龙江" pyName="heilongjiang" cityname="哈尔滨" state1="4" state2="1" stateDetailed="雷阵雨转多云" tem1="21"
				  tem2="9" windState="西南风3-4级转西风微风级"><weather>18</weather></city>
			<city quName="吉林" pyName="jilin" cityname="长春" state1="7" state2="1" stateDetailed="小雨转多云" tem1="20" tem2="13"
				  windState="西风转东南风微风级"><weather>17</weather></city>
	</china>
	<china dn="hour">
		<weather>
			<city quName="黑龙江" pyName="heilongjiang" cityname="哈尔滨" state1="4" state2="1" stateDetailed="雷阵雨转多云" tem1="20"
				  tem2="9" windState="西南风3-4级转西风微风级">15</city>
		</weather>
		<weather>
			<city quName="吉林" pyName="jilin" cityname="长春" state1="7" state2="1" stateDetailed="小雨转多云" tem1="20" tem2="10"
				  windState="西风转东南风微风级">16</city>
		</weather>
	</china>
<root>
`

func AssertNoError(t *testing.T, err error) {
	if !assert.NoError(t, err) {
		os.Exit(1)
	}
}

func TestMetricConfig_GetMetricByXml(t *testing.T) {
	var err error
	mcs := []MetricConfig{{
		Name:       "test_xml1",
		MetricType: Gauge,
		Match: MetricMatch{
			Datapoint: "//china[@dn='hour']/weather/city",
			Labels: map[string]string{
				"__name__":  `{{ (.SelectAttr "quName").Value }}`,
				"__value__": `{{ .Text }}`,
			},
		},
	}, {
		Name:       "test_xml2",
		MetricType: Gauge,
		Match: MetricMatch{
			Datapoint: "//china[@dn='day']/weather/city",
			Labels: map[string]string{
				"__name__":  `(.SelectAttr "quName").Value }}`,
				"__value__": `{{ .Text }}`,
			},
		},
	}, {
		Name:       "test_xml3",
		MetricType: Gauge,
		Match: MetricMatch{
			Datapoint: "//china[@dn='week']/city/weather",
			Labels: map[string]string{
				"__name__":  `{{ ((.FindElement "../").SelectAttr "quName").Value }}`,
				"__value__": `{{ .Text }}`,
			},
		},
	}}
	for _, mc := range mcs {
		err = mc.BuildTemplate("")
		AssertNoError(t, err)
		metrics := make(chan Metric, 3)
		logger := log.NewLogfmtLogger(os.Stderr)
		go func() {
			mc.GetMetricByXml(logger, []byte(xmlContent), mc.RelabelConfigs, metrics)
			close(metrics)
		}()
		for metric := range metrics {
			m, err := metric.getMetric()
			AssertNoError(t, err)
			dtoMetric := dto.Metric{}
			AssertNoError(t, m.Write(&dtoMetric))
			fmt.Println(dtoMetric.String())
		}
	}
}

var textContent = `
@[server5]/cpu=12/memory=14359738368/ip=3.3.3.3/hostname=database2! # database2监控数据
@[server6]/cpu=16/memory=34359738368/ip=4.4.4.4/hostname=gateway-server2!  # gateway-server2监控数据
`

func TestMetricConfig_GetMetricByRegex(t *testing.T) {
	var err error
	mcs := []MetricConfig{{
		Name:       "info",
		MetricType: Gauge,
		RelabelConfigs: RelabelConfigs{&RelabelConfig{
			TargetLabel: "__name__",
			Action:      Replace,
			Separator:   ";",
			Regex:       MustNewRegexp(`(.*)`),
			Replacement: "info",
		}, &RelabelConfig{
			TargetLabel: "__value__",
			Action:      Replace,
			Separator:   ";",
			Regex:       MustNewRegexp(`(.*)`),
			Replacement: "1",
		}},
		Match: MetricMatch{
			Datapoint: "@\\[(?P<name>[^[]+)]/.+/ip=(?P<ip>[\\d.]+)/hostname=(?P<hostname>.+?)!",
		},
	}, {
		Name:       "memory",
		MetricType: Gauge,
		RelabelConfigs: RelabelConfigs{&RelabelConfig{
			TargetLabel: "__name__",
			Action:      Replace,
			Separator:   ";",
			Regex:       MustNewRegexp(`(.*)`),
			Replacement: "memory",
		}},
		Match: MetricMatch{
			Datapoint: "@\\[(?P<name>.+?)].*!",
			Labels: map[string]string{
				"__value__": "memory=(?P<__value__>[\\d.]+)",
			},
		},
	}, {
		Name:       "cpu",
		MetricType: Gauge,
		RelabelConfigs: RelabelConfigs{&RelabelConfig{
			SourceLabels: []model.LabelName{"__raw__"},
			TargetLabel:  "__value__",
			Action:       Replace,
			Separator:    ";",
			Regex:        MustNewRegexp(`.*cpu=([\d.]+).*`),
			Replacement:  "$1",
		}, &RelabelConfig{
			SourceLabels: []model.LabelName{"__raw__"},
			TargetLabel:  "name",
			Action:       Replace,
			Separator:    ";",
			Regex:        MustNewRegexp(`.*@\[(.+?)].*`),
			Replacement:  "$1",
		}, &RelabelConfig{
			TargetLabel: "__name__",
			Action:      Replace,
			Separator:   ";",
			Regex:       MustNewRegexp(`(.*)`),
			Replacement: "cpu",
		}},
		Match: MetricMatch{
			Datapoint: "@.*!",
			Labels: map[string]string{
				"__raw__": `.*`,
			},
		},
	}}
	for _, mc := range mcs {
		err = mc.BuildRegexp("")
		AssertNoError(t, err)
		metrics := make(chan Metric, 3)
		logger := log.NewLogfmtLogger(os.Stderr)
		go func() {
			mc.GetMetricByRegex(logger, []byte(textContent), mc.RelabelConfigs, metrics)
			close(metrics)
		}()
		for metric := range metrics {
			m, err := metric.getMetric()
			AssertNoError(t, err)
			dtoMetric := dto.Metric{}
			AssertNoError(t, m.Write(&dtoMetric))
			fmt.Println(dtoMetric.String())
		}
	}
}
