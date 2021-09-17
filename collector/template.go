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
