package internal

import (
	"bytes"
	"html/template"
)

// RenderTemplates returns an []string to render the templates
func RenderTemplates(templates []string, data interface{}) (args []string, err error) {
	var t *template.Template

	for _, tpl := range templates {
		t, err = template.New(tpl).Parse(tpl)
		if err != nil {
			args = nil
			return
		}

		buf := &bytes.Buffer{}
		err = t.Execute(buf, data)
		if err != nil {
			args = nil
			return
		}
		args = append(args, buf.String())
	}

	return
}
