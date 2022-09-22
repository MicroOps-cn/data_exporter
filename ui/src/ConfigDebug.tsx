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

import React, { FC, useEffect, useState } from "react";
import { Button, Col, Input, List, message, Radio, Row, Space } from "antd";
import "./ConfigDebug.css";
import { CodeBox } from "./components/CodeBox";
import {
  ConfigPlaceholder,
  DefaultConfig,
  DemoConfig,
  DemoConfigs,
} from "./components/Demo";
import { getDatapoints, getMetrics, loadData } from "./services/request";
import { DelayedRun } from "./utils/delayed";
import { ArrowDownOutlined } from "@ant-design/icons";
import LabelMatchEditableTable, {
  DataSourceType,
} from "./components/LabelMatchEditableTable";
import yaml from "js-yaml";

const ConfigDebugContainer: FC = () => {
  const [config, setConfig] = useState<string>(DefaultConfig);
  const [data, setData] = useState<string>("");
  const [datapointMatchRule, setDatapointMatchRule] = useState<string>();
  const [matchRules, setMatchRules] = useState<DataSourceType[]>([]);
  const [datapoints, setDatapoints] = useState<string[]>();
  const [mode, setMode] = useState<"xml" | "json" | "yaml" | "regex">("json");
  const [demoConfig, setDemoConfig] = useState<DemoConfig>();
  const [metrics, setMetrics] = useState<string>("");
  useEffect(() => {
    if (DemoConfigs[mode]) {
      setDemoConfig(DemoConfigs[mode]);
    }
  }, [mode]);

  useEffect(() => {
    const conf = { ...(yaml.load(config) as Config) };
    const relabel_configs: RelabelConfig[] = [];
    conf.collects?.forEach((collect) => {
      relabel_configs.push(...(collect.relabel_configs ?? []));
      collect.datasource?.forEach((ds) => {
        relabel_configs.push(...(ds.relabel_configs ?? []));
      });
      collect.metrics?.forEach((metrics) => {
        relabel_configs.push(...(metrics.relabel_configs ?? []));
      });
    });
    const metric_config = conf.collects?.[0].metrics?.[0];
    if (metric_config && datapoints && relabel_configs) {
      getMetrics({ datapoints, relabel_configs, metric_config })
        .then((ret) => {
          setMetrics(ret);
        })
        .catch((reason) => {
          message.error(reason);
        });
    } else {
      setMetrics("");
    }
  }, [config, datapoints]);
  useEffect(() => {
    if (data && datapointMatchRule) {
      const match: { [key: string]: string } = {};
      matchRules.forEach((item) => {
        match[item.name ?? ""] = item.match ?? "";
      });
      DelayedRun(() => {
        getDatapoints({
          data: data,
          rule: datapointMatchRule,
          mode: mode,
          label_match: match,
        })
          .then((ret) =>
            setDatapoints(
              ret?.map((item) =>
                typeof item === "string" ? item : JSON.stringify(item)
              ) ?? []
            )
          )
          .catch((reason) => {
            message.error(`${reason}`);
          });
      })();
    } else {
      setDatapoints(undefined);
    }
  }, [data, datapointMatchRule, mode, matchRules]);

  const changeConfig = (
    f: (
      conf: Config,
      collect: CollectConfig,
      datasource: DatasourceConfig,
      metrics: MetricConfig
    ) => void
  ) => {
    const c = { ...(yaml.load(config) as Config) };
    if (!c.collects || c.collects.length === 0) c.collects = [{}];
    if (!c.collects[0].datasource || c.collects[0].datasource.length === 0) {
      c.collects[0].datasource = [{}];
    }
    if (!c.collects[0].metrics || c.collects[0].metrics.length === 0) {
      c.collects[0].metrics = [{}];
    }
    f(c, c.collects[0], c.collects[0].datasource[0], c.collects[0].metrics[0]);
    setConfig(yaml.dump(c));
  };

  return (
    <div className="App">
      <div className="ConfigBox">
        <CodeBox
          mode={"yaml"}
          placeholder={ConfigPlaceholder}
          editorDidMount={(editor) => {
            console.log(editor);
            editor.focus();
          }}
          onBlur={(editor) => {
            const currentConf = editor.getValue();
            const confObj = yaml.load(currentConf) as Config;
            const { collects } = confObj;
            if (Array.isArray(collects) && collects.length > 0) {
              const collect = collects[0];
              const { datasource, metrics, data_format } = collect;
              if (data_format) setMode(data_format);
              if (datasource && datasource?.length > 0) {
                collect.datasource = [datasource[0]];
              }
              if (metrics && metrics?.length > 0) {
                collect.metrics = [metrics[0]];
                const dp = metrics[0].match?.datapoint;
                if (dp) setDatapointMatchRule(dp);
                const matchLabels = metrics[0].match?.labels;
                if (matchLabels) {
                  const rules: DataSourceType[] = [];
                  for (const key in matchLabels) {
                    if (
                      Object.prototype.hasOwnProperty.call(matchLabels, key)
                    ) {
                      rules.push({
                        id: key,
                        name: key,
                        match: matchLabels[key],
                      });
                    }
                  }
                  setMatchRules(rules);
                }
              }

              confObj.collects = [collect];
            }
            const confStr = yaml.dump(confObj);
            if (confStr !== currentConf) {
              editor.setValue(confStr);
            }
            setConfig(confStr);
          }}
          value={config}
          setValue={setConfig}
        />
      </div>
      <Row justify="space-evenly">
        <Col span={8}></Col>
        <Col span={14}>
          <Space className="DataMatchModeSelect">
            <Radio.Group
              onChange={(e) => {
                setMode(e.target.value);
                changeConfig((_, collect) => {
                  collect.data_format = e.target.value;
                });
              }}
              value={mode}
              defaultValue="json"
              style={{ marginTop: 16 }}
            >
              <Radio.Button value="json">JSON</Radio.Button>
              <Radio.Button value="yaml">Yaml</Radio.Button>
              <Radio.Button value="xml">XML</Radio.Button>
              <Radio.Button value="regex">Regex</Radio.Button>
            </Radio.Group>
            <Button
              onClick={() => {
                const c = { ...(yaml.load(config) as Config) };
                if (c.collects?.[0].datasource) {
                  loadData(c.collects[0].datasource[0])
                    .then((data) => {
                      setData(data);
                    })
                    .catch((reason) => {
                      message.error(`${reason}`);
                    });
                }
              }}
              style={{ marginTop: 16 }}
            >
              Load data
            </Button>
          </Space>
          <CodeBox
            value={data}
            setValue={setData}
            mode={mode}
            placeholder={demoConfig?.data}
          />
          <Input
            className="Datapoint Control"
            placeholder={demoConfig?.datapoint}
            value={datapointMatchRule}
            onChange={(e) => {
              setDatapointMatchRule(e.currentTarget.value);
              changeConfig((_, __, ___, metric) => {
                if (!metric.match) {
                  metric.match = {};
                }
                metric.match.datapoint = e.currentTarget.value;
              });
            }}
          />
          <LabelMatchEditableTable
            className="MatchRoles Control"
            values={matchRules}
            onChange={(val) => {
              setMatchRules(val);
              changeConfig((_, __, ___, metric) => {
                if (!metric.match) metric.match = {};
                const newLabelMatch: { [key: string]: string } = {};
                val.forEach((item) => {
                  newLabelMatch[item.name ?? ""] = item.match ?? "";
                });
                metric.match.labels = newLabelMatch;
              });
            }}
          />
          <ArrowDownOutlined
            style={{ display: "flex", justifyContent: "center" }}
          />
          <List
            header={"Matched data results: "}
            className="Datapoints Control"
            size="small"
            bordered
            locale={{
              emptyText: "No data",
            }}
            dataSource={datapoints ?? demoConfig?.datapoints}
            renderItem={(item) => <List.Item>{item}</List.Item>}
          />
          <ArrowDownOutlined
            style={{ display: "flex", justifyContent: "center" }}
          />
          <div className="MetricBox Control">
            <CodeBox
              value={metrics}
              mode={mode}
              placeholder={"data of prometheus format."}
            />
          </div>
        </Col>
      </Row>
    </div>
  );
};
export default ConfigDebugContainer;
