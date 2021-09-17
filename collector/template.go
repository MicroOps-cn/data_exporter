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
	"bytes"
	"text/template"
)

type Template struct {
	*template.Template
}

func (t *Template) Funcs(funcMap template.FuncMap) *Template {
	t.Template = t.Template.Funcs(funcMap)
	return t
}
func (t *Template) Execute(data interface{}) ([]byte, error) {
	buf := bytes.Buffer{}
	err := t.Template.Execute(&buf, data)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
func (t *Template) ExecuteTemplate(name string, data interface{}) ([]byte, error) {
	buf := bytes.Buffer{}
	err := t.Template.ExecuteTemplate(&buf, name, data)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
func NewTemplate(name string, text string) (*Template, error) {
	if tmpl, err := template.New(name).Parse(text); err != nil {
		return nil, err
	} else {
		return &Template{Template: tmpl}, nil
	}
}
