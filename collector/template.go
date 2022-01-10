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
	"fmt"
	"gopkg.in/yaml.v3"
	"regexp"
	"strconv"
	"strings"
	"text/template"
	"time"
)

type Template struct {
	*template.Template
	original string
}

var safeFuncMap = template.FuncMap{
	"toUpper": strings.ToUpper,
	"toLower": strings.ToLower,
	"title":   strings.Title,
	"reReplaceAll": func(pattern, repl, text string) string {
		re := regexp.MustCompile(pattern)
		return re.ReplaceAllString(text, repl)
	},
	"now": time.Now,
	"utcNow": func() time.Time {
		return time.Now().UTC()
	},
	"parseInt": func(base, bitSize int, s string) int64 {
		val, err := strconv.ParseInt(s, base, bitSize)
		if err != nil {
			panic(err)
		}
		return val
	},
	"parseFloat": func(bitSize int, s string) float64 {
		val, err := strconv.ParseFloat(s, bitSize)
		if err != nil {
			panic(err)
		}
		return val
	},
	"formatInt": func(base int, i int64) string {
		return strconv.FormatInt(i, base)
	},
	"formatFloat": func(fmt byte, prec, bitSize int, f float64) string {
		return strconv.FormatFloat(f, fmt, prec, bitSize)
	},
	"toString": func(v interface{}) string {
		return fmt.Sprintf("%v", v)
	},

	"trimSpace": strings.TrimSpace,
	"trimLeft": func(cutset, s string) string {
		return strings.TrimLeft(s, cutset)
	},
	"trimRight": func(cutset, s string) string {
		return strings.TrimRight(s, cutset)
	},
	"trimPrefix": func(prefix, s string) string {
		return strings.TrimPrefix(s, prefix)
	},
	"trimSuffix": func(suffix, s string) string {
		return strings.TrimSuffix(s, suffix)
	},
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
	if tmpl, err := template.New(name).Funcs(safeFuncMap).Parse(text); err != nil {
		return nil, err
	} else {
		return &Template{Template: tmpl, original: text}, nil
	}
}
func MustNewTemplate(name string, text string) Template {
	if tmpl, err := NewTemplate(name, text); err != nil {
		panic(err)
	} else {
		return *tmpl
	}
}

func (t Template) MarshalYAML() (interface{}, error) {
	if t.original != "" {
		return t.original, nil
	}
	return nil, nil
}

func (t *Template) UnmarshalYAML(value *yaml.Node) error {
	var tmplStr string
	if err := value.Decode(&tmplStr); err != nil {
		return err
	} else if len(tmplStr) != 0 {

		if tmpl, err := NewTemplate("", tmplStr); err != nil {
			return err
		} else {
			*t = *tmpl
		}
	}
	return nil
}
