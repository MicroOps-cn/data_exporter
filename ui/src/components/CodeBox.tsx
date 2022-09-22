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

import { Select } from "antd";
import { useEffect, useState } from "react";
import { Controlled as CodeMirror, IControlledCodeMirror } from "react-codemirror2";

import "codemirror/lib/codemirror.css";
import "codemirror/theme/monokai.css";
import "codemirror/mode/xml/xml";
import "codemirror/mode/yaml/yaml";
import "codemirror/mode/javascript/javascript";
import "codemirror/addon/display/placeholder";
// import 'codemirror/mode/json/json';
import "./CodeBox.css";

interface CodeBoxProps extends Omit<IControlledCodeMirror,"onBeforeChange"> {
  mode?: "xml" | "json" | "yaml" | "regex";
  placeholder?: string;
  setValue?: (val: string) => void;
}

export const CodeBox: React.FC<CodeBoxProps> = ({className, value, setValue, mode = "json", placeholder = "",...props }) => {
  const [editorMode, setEditorMode] = useState<string>("strings");

  useEffect(() => {
    switch (mode) {
      case "xml":
      case "yaml":
        setEditorMode(mode);
        break;
      case "json":
        setEditorMode("application/json");
        break;
    }
  }, [mode]);

  return (
    <div>
      <div className="CodeBoxToolbar">
        <Select value={editorMode} size="middle" onChange={setEditorMode}>
          <option value="xml">xml</option>
          <option value="yaml">yaml</option>
          <option value="application/json">json</option>
          <option value="strings">strings</option>
        </Select>
      </div>
      <CodeMirror
        className={`${className??""} CodeBox`}
        value={value}
        onBeforeChange={(editor, data, val) => {
          setValue?.(val);
        }}
        options={{
          placeholder: placeholder,
          mode: editorMode,
          theme: "monokai",
          lineNumbers: false,
          readOnly: setValue?false:true
        }}
        {...props}
      />
    </div>
  );
};
