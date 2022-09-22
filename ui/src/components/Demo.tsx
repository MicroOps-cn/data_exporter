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

export interface DemoConfig {
  data: string;
  datapoint: string;
  datapoints: string[];
}

export const DemoConfigs: { [key: string]: DemoConfig } = {
  json: {
    data: `// Please input raw json data, like this:
{
  "data": {
    "server1": {
      "metrics": {
        "CPU": "0x10",
        "Memory": "0x1000000000"
      }
    },
    "server2": {
      "metrics": {
        "CPU": "0x10",
        "Memory": "0x1000000000"
      }
    }
  },
  "code": 0
}`,
    datapoint:
      "Please input datapoint match rule, like this: data|@expand|@expand|@to_entries:name:value",
    datapoints: [
      `{"name": "server1.metrics.Memory", "value": "0x1000000000"}`,
      `{"name": "server2.metrics.CPU", "value": "0x10"}`,
      `{"name": "server2.metrics.Memory", "value": "0x1000000000"}`,
      `{"name": "server1.metrics.CPU", "value": "0x10"}`,
    ],
  },
  regex: {
    data: `// Please input raw data, like this:
[server4]
cpu=12
memory=24359738368
hostname=database1
ip=1.1.1.1
[server3]
cpu=16
memory=24359738368
hostname=gateway-server1
ip=2.2.2.2`,
    datapoint:
      "Please input datapoint match rule, like this: (?ms:\\[(?P<name>[^]]+)][^[]+)",
    datapoints: [
      `{"__line__":"[server4]\ncpu=12\nmemory=24359738368\nhostname=database1\nip=1.1.1.1\n","name":"server4"}`,
      `{"__line__":"[server3]\ncpu=16\nmemory=24359738368\nhostname=gateway-server1\nip=2.2.2.2","name":"server3"}`,
    ],
  },
  yaml: {
    data: `// Please input raw yaml data, like this:
data:
  server1:
    metrics:
      CPU: '0x10'
      Memory: '0x1000000000'
  server2:
    metrics:
      CPU: '0x10'
      Memory: '0x1000000000'
  code: 0`,
    datapoint:
      "Please input datapoint match rule, like this: data|@expand|@expand|@to_entries:name:value",
    datapoints: [
      `{"name": "server1.metrics.Memory", "value": "0x1000000000"}`,
      `{"name": "server2.metrics.CPU", "value": "0x10"}`,
      `{"name": "server2.metrics.Memory", "value": "0x1000000000"}`,
      `{"name": "server1.metrics.CPU", "value": "0x10"}`,
    ],
  },
  xml: {
    data: `// Please input raw xml data, like this:
<root>
  <china dn="day">
    <weather>
      <city quName="黑龙江" pyName="heilongjiang" cityname="哈尔滨" state1="4" state2="1" stateDetailed="雷阵雨转多云"
            tem1="20"
            tem2="9" windState="西南风3-4级转西风微风级">15
      </city>
      <city quName="吉林" pyName="jilin" cityname="长春" state1="7" state2="1" stateDetailed="小雨转多云" tem1="20"
            tem2="10"
            windState="西风转东南风微风级">16
      </city>
      <city quName="辽宁" pyName="liaoning" cityname="沈阳" state1="3" state2="1" stateDetailed="阵雨转多云" tem1="21"
            tem2="15" windState="北风3-4级">18
      </city>
      <city quName="海南" pyName="hainan" cityname="海口" state1="1" state2="1" stateDetailed="多云" tem1="32" tem2="26"
            windState="微风">30
      </city>
    </weather>
  </china>
  <china dn="week">
    <city quName="黑龙江" pyName="heilongjiang" cityname="哈尔滨" state1="4" state2="1" stateDetailed="雷阵雨转多云" tem1="21"
          tem2="9" windState="西南风3-4级转西风微风级">
      <weather>18</weather>
    </city>
    <city quName="吉林" pyName="jilin" cityname="长春" state1="7" state2="1" stateDetailed="小雨转多云" tem1="20" tem2="13"
          windState="西风转东南风微风级">
      <weather>17</weather>
    </city>
  </china>
  <china dn="hour">
    <weather>
      <city quName="黑龙江" pyName="heilongjiang" cityname="哈尔滨" state1="4" state2="1" stateDetailed="雷阵雨转多云"
            tem1="20"
            tem2="9" windState="西南风3-4级转西风微风级">15
      </city>
    </weather>
    <weather>
      <city quName="吉林" pyName="jilin" cityname="长春" state1="7" state2="1" stateDetailed="小雨转多云" tem1="20"
            tem2="10"
            windState="西风转东南风微风级">16
      </city>
    </weather>
  </china>
</root>
    `,
    datapoint:
      "Please input datapoint match rule, like this: //china[@dn='week']/city/weather",
    datapoints: [
      `{"name": "server1.metrics.Memory", "value": "0x1000000000"}`,
      `{"name": "server2.metrics.CPU", "value": "0x10"}`,
      `{"name": "server2.metrics.Memory", "value": "0x1000000000"}`,
      `{"name": "server1.metrics.CPU", "value": "0x10"}`,
    ],
  },
};

export const DefaultConfig = `collects:
- name: test-http
  relabel_configs: []
  data_format: json
  datasource:
    - type: file
      url: my_data.json
      name: simple-http-data
      allow_replace: true
  metrics:
    - name: Point1
      relabel_configs:
        - target_label: __namespace__
          replacement: node_server
        - source_labels:
            - __name__
          target_label: name
          regex: ([^.]+)\\.metrics\\..+
          replacement: $1
          action: replace
        - source_labels:
            - __name__
          target_label: __name__
          regex: '[^.]+\\.metrics\\.(.+)'
          replacement: $1
          action: replace
        - source_labels:
            - __value__
          target_label: __value__
          action: templexec
          template: '{{ .|parseInt 0 64 }}'
        - target_label: value
          replacement: ''
      match:
        datapoint: data|@expand|@expand|@to_entries:name:value
        labels:
          __value__: value
          __name__: name`

export const ConfigPlaceholder = `Please enter the configuration in yaml format. During debugging, only the first metric configuration in the first collection configuration will be resolved (collections[0].metrics[0])
collects:
  - name: test-http
    relabel_configs: []
    data_format: json
    datasource:
      - type: file
        url: my_data.json
        name: simple-http-data
        allow_replace: true
    metrics:
      - name: Point1
        relabel_configs:
          - target_label: __namespace__
            replacement: node_server
          - source_labels:
              - __name__
            target_label: name
            regex: ([^.]+)\\.metrics\\..+
            replacement: $1
            action: replace
          - source_labels:
              - __name__
            target_label: __name__
            regex: '[^.]+\\.metrics\\.(.+)'
            replacement: $1
            action: replace
          - source_labels:
              - __value__
            target_label: __value__
            action: templexec
            template: '{{ .|parseInt 0 64 }}'
        match:
          datapoint: data|@expand|@expand|@to_entries:name:value
          labels:
            __value__: value
            __name__: name

`
