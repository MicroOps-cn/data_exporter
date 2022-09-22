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

declare interface RelabelConfig {
  source_labels?: string[];
  regex?: string;
  target_label?: string;
  replacement?: string;
}

declare interface DatasourceConfig {
  name?: string;
  relabel_configs?: RelabelConfig[];
  type?: "file"|"tcp"|"udp"|"http"
  url?: string
}

declare interface MetricConfig {
  name?: string;
  relabel_configs?: RelabelConfig[];
  metric_type?: "gauge" | "counter" | "histogram"
  match?: MetricMatchConfig;
}

declare interface MetricMatchConfig {
  datapoint?: string;
  labels?: { [key: string]: string }
}

declare interface CollectConfig {
  name?: string;
  data_format?: "regex" | "json" | "yaml" | "xml";
  relabel_configs?: RelabelConfig[];
  datasource?: DatasourceConfig[];
  metrics?: MetricConfig[];
}

declare interface Config {
  collects?: CollectConfig[]
}