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

const proxy = require('http-proxy-middleware')

module.exports = function(app){
	app.use(
		proxy.createProxyMiddleware(`/-/ui/api/`,{ //遇见/api1前缀的请求，就会触发该代理配置
			target:'http://localhost:9116/', //请求转发给谁
			changeOrigin:true,//控制服务器收到的请求头中Host的值
			// pathRewrite:'/api/',
		}),
	)
}