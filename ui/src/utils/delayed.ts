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

class RunState{
    private delay: {[key:string] :number}
    private  lastParams: {[key:string] :number}
    private constructor(){
        this.delay={}
        this.lastParams={}
    }
    private static state:RunState;
    public static GetInstance(){
        if(this.state){
            return this.state;
        }
        else{
            this.state=new RunState();
            return this.state;
        }
    }
    public SetDelay(name:string,value: number){
        this.delay[name] = value
    }
    public GetDelay(name:string){
        return this.delay[name]
    }
    public SetLastParams(name:string,value: any){
        this.lastParams[name] = value
    }
    public GetLastParams(name:string){
        return this.lastParams[name]
    }
}

export const DelayedRun =  (handler: (params: any) => void, timeout?: number) => {
    const t = timeout !== undefined ? timeout : 1
    const state = RunState.GetInstance()
    console.log(handler.name)
    return (params?: any) => {
      if (state.GetDelay(handler.name) === undefined) {
        state.SetDelay(handler.name, 0)
      }
      state.SetDelay(handler.name,state.GetDelay(handler.name)+ t)
      setTimeout(async function () {
        params = params !== undefined ? params : ""
        state.SetDelay(handler.name,state.GetDelay(handler.name)- t)
        if (state.GetDelay(handler.name) === 0) {
          if (params !== state.GetLastParams(handler.name)) {
            state.SetLastParams(handler.name,params)
          }
          handler(params)
        }
      }, t * 1000)
    }
}

// export const DelayedAsyncRun = async (handler: (params: any) => void, timeout?: number) => {
//     const t = timeout !== undefined ? timeout : 1
//     const { delay, lastKey } = searchState.current
//     return (params?: string) => {
//       if (delay[handler.name] === undefined) {
//         delay[handler.name] = 0
//       }
//       delay[handler.name] = delay[handler.name] + t
//       setTimeout(async function () {
//         params = params !== undefined ? params : ""
//         delay[handler.name] = delay[handler.name] - t;
//         if (delay[handler.name] == 0) {
//           if (params !== lastKey[handler.name]) {
//             handler(params)
//             lastKey[handler.name] = params
//           }
//         }
//       }, t * 1000)
//     }
// }
