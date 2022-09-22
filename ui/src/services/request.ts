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

import jsyaml from "js-yaml";
export type RequestResponse<T = any> = {
  data: T;
  response: Response;
};
interface Request<R = false> {
  <T = any>(url: RequestInfo | URL, init?: RequestInit, decodeRespose?: boolean): R extends true ? Promise<RequestResponse<T>> : Promise<T>;
}

const request: Request = async (url: RequestInfo | URL, init?: RequestInit, decodeRespose: boolean = true) => {
  const resp = await fetch(url, init)
  if (resp.status !== 200) {
    throw new Error(`Server response exception: code=${resp.status}`);
  }
  if(decodeRespose){
    return resp.json()
  }
  return await resp.text()
}

export const getDatapoints = (body: { data: string, rule: string, mode: string, label_match: { [key: string]: string } }) => {
  return request<string[] | { [key: string]: any }[]>("../api/datapoints", {
    method: "POST",
    body: JSON.stringify(body)
  })
}

export const getMetrics = (body: { datapoints: any[], relabel_configs: RelabelConfig[], metric_config: MetricConfig }) => {
  return request<string>("../api/metrics", {
    method: "POST",
    body: JSON.stringify(body)
  },false)
}


export const loadData = async (body: CollectConfig) => {
  return request<string>("../api/load/data", {
    method: "POST",
    body: jsyaml.dump(body)
  },false)
}