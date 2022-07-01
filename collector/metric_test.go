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
	"github.com/go-kit/log"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		metrics := make(chan MetricGenerator, 3)
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
			t.Log(dtoMetric.String())
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
			Replacement: "0x11",
		}, &RelabelConfig{
			TargetLabel:  "__value__",
			Action:       TemplateExecute,
			SourceLabels: model.LabelNames{"__value__"},
			Separator:    ";",
			Template:     MustNewTemplate("", `{{ .|parseInt 0 64 |toString }}`),
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
		metrics := make(chan MetricGenerator, 3)
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
			t.Log(dtoMetric.String())
		}
	}
}

const valuesXmlContent = `
<?xml version="1.0" encoding="UTF-8" standalone="no"?>
<measCollecFile xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xsi:schemaLocation="http://www.3gpp.org/ftp/specs/archive/32_series/32.435#measCollec http://www.3gpp.org/ftp/specs/archive/32_series/32.435#measCollec" xmlns="http://www.3gpp.org/ftp/specs/archive/32_series/32.435#measCollec">
   <fileHeader fileFormatVersion="32.435 V7.0" vendorName="XXXX XXXX.">
      <fileSender localDn="SubNetwork=ANIEMS"/>
      <measCollec beginTime="2022-03-17T10:00:00+00:00"/>
   </fileHeader>
   <measData>
      <managedElement localDn="ManagedElement=ORAN-XXXX-TB" swVersion="V4.0.4g_132_oran.1"/>
      <measInfo measInfoId="FDDL">
         <measTypes>VS.FDDL.TrnsmssnMode2Nbr</measTypes>
         <measValue measObjLdn="ENBFunction=ORAN-XXXX-TB,EUtranCellFDD=TL08-123456-31">
            <measResults>101 200 300 90 30</measResults>
         </measValue>
         <measValue measObjLdn="ENBFunction=ORAN-XXXX-TB,EUtranCellFDD=TL08-123456-11">
            <measResults>10 20 30 40 50</measResults>
         </measValue>
         <measValue measObjLdn="ENBFunction=ORAN-XXXX-TB,EUtranCellFDD=TL08-123456-21">
            <measResults>11 12 13 14 15</measResults>
         </measValue>
         <measValue measObjLdn="ENBFunction=ORAN-XXXX-TB,EUtranCellFDD=TL21-123456-11">
            <measResults>1 2 3 4 5</measResults>
         </measValue>
         <measValue measObjLdn="ENBFunction=ORAN-XXXX-TB,EUtranCellFDD=TL21-123456-31">
            <measResults>6 7 8 9 10</measResults>
         </measValue>
         <measValue measObjLdn="ENBFunction=ORAN-XXXX-TB,EUtranCellFDD=TL21-123456-21">
            <measResults>9 8 7 6 5</measResults>
         </measValue>
      </measInfo>
      <measInfo measInfoId="RRC">
         <measTypes>VS.RRC.RedirEutranSuccNbr VS.RRC.RedirEutranAttA2Nbr VS.RRC.EutrantoNRRedirAttNbr VS.RRC.RedirEutranAttUnknownPlmnNbr</measTypes>
         <measValue measObjLdn="ENBFunction=ORAN-XXXX-TB,EUtranCellFDD=TL08-123456-31">
            <measResults>10 20 30 40</measResults>
         </measValue>
         <measValue measObjLdn="ENBFunction=ORAN-XXXX-TB,EUtranCellFDD=TL08-123456-11">
            <measResults>60 70 80 90</measResults>
         </measValue>
         <measValue measObjLdn="ENBFunction=ORAN-XXXX-TB,EUtranCellFDD=TL08-123456-21">
            <measResults>50 40 30 20</measResults>
         </measValue>
         <measValue measObjLdn="ENBFunction=ORAN-XXXX-TB,EUtranCellFDD=TL21-123456-11">
            <measResults>10 20 20 30</measResults>
         </measValue>
         <measValue measObjLdn="ENBFunction=ORAN-XXXX-TB,EUtranCellFDD=TL21-123456-31">
            <measResults>40 30 20 10</measResults>
         </measValue>
         <measValue measObjLdn="ENBFunction=ORAN-XXXX-TB,EUtranCellFDD=TL21-123456-21">
            <measResults>90 20 30 10</measResults>
         </measValue>
      </measInfo>
</measCollecFile>
`

func TestMetricConfig_GetMetricByValues(t *testing.T) {
	var err error
	mcs := []MetricConfig{{
		Name:       "test_xml1",
		MetricType: Gauge,
		Match: MetricMatch{
			Datapoint: "//measData/measInfo/measValue/measResults",
			Labels: map[string]string{
				"__values__":                  `{{ .Text }}`,
				"__values_index__":            `{{ (.FindElement "../../measTypes").Text }}`,
				"__values_separator__":        " ",
				"__values_index_label_name__": "type",
				"__values_index_separator__":  " ",
				"name":                        `{{ ((.FindElement "../").SelectAttr "measObjLdn").Value }}`,
				"meas_info_id":                `{{ ((.FindElement "../../").SelectAttr "measInfoId").Value }}`,
				"__time__":                    `{{ ((.FindElement "../../../../fileHeader/measCollec").SelectAttr "beginTime").Value }}`,
			},
		},
	}}
	for _, mc := range mcs {
		err = mc.BuildTemplate("")
		AssertNoError(t, err)
		metrics := make(chan MetricGenerator, 3)
		logger := log.NewLogfmtLogger(os.Stderr)
		go func() {
			mc.GetMetricByXml(logger, []byte(valuesXmlContent), mc.RelabelConfigs, metrics)
			close(metrics)
		}()
		var errors []error
		var dtoMetrics []dto.Metric
		for metric := range metrics {
			ms, errs := metric.getMetrics()
			for _, err = range errs {
				if err != nil {
					errors = append(errors, err)
				}
			}
			for _, m := range ms {
				if m != nil {
					dtoMetric := dto.Metric{}
					AssertNoError(t, m.Write(&dtoMetric))
					require.Contains(t, dtoMetric.String(), `label:<name:"type" value:`)
					dtoMetrics = append(dtoMetrics, dtoMetric)
				}
			}
		}
		require.Equal(t, len(dtoMetrics), 24)
		require.Equal(t, len(errors), 6)
	}
}
