package cardrender

import (
	"bytes"
	"embed"
	"fmt"
	"sort"
	"strings"
	"text/template"
)

//go:embed templates/*.html
var templatesFS embed.FS

var parsedTemplates map[string]*template.Template

var funcMap = template.FuncMap{
	"add": func(a, b int) int { return a + b },
	"percent": func(value, max interface{}) float64 {
		v := toFloat64(value)
		m := toFloat64(max)
		if m == 0 {
			return 0
		}
		p := (v / m) * 100
		if p > 100 {
			p = 100
		}
		return p
	},
}

func toFloat64(v interface{}) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case string:
		return 0
	default:
		return 0
	}
}

func init() {
	parsedTemplates = make(map[string]*template.Template)

	entries, err := templatesFS.ReadDir("templates")
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".html") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".html")
		data, err := templatesFS.ReadFile("templates/" + e.Name())
		if err != nil {
			continue
		}
		tmpl, err := template.New(name).Funcs(funcMap).Parse(string(data))
		if err != nil {
			continue
		}
		parsedTemplates[name] = tmpl
	}
}

// RenderTemplate fills a named template with data and returns complete HTML.
func RenderTemplate(name string, data interface{}) (string, error) {
	tmpl, ok := parsedTemplates[name]
	if !ok {
		return "", newRenderError(ErrInvalidTemplate,
			fmt.Sprintf("template %q not found, available: %s", name, strings.Join(ListTemplates(), ", ")),
			nil)
	}

	data = preprocessData(name, data)

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", newRenderError(ErrInvalidTemplate,
			fmt.Sprintf("failed to execute template %q", name), err)
	}
	return buf.String(), nil
}

// preprocessData computes derived fields that templates need (e.g. maxValue
// for the ranking template) so template logic stays simple.
func preprocessData(name string, data interface{}) interface{} {
	m, ok := data.(map[string]interface{})
	if !ok {
		return data
	}
	if name == "ranking" {
		items, _ := m["items"].([]interface{})
		var maxVal float64
		for _, item := range items {
			if im, ok := item.(map[string]interface{}); ok {
				v := toFloat64(im["value"])
				if v > maxVal {
					maxVal = v
				}
			}
		}
		m["maxValue"] = maxVal
	}
	return m
}

// ListTemplates returns sorted names of all available templates.
func ListTemplates() []string {
	names := make([]string, 0, len(parsedTemplates))
	for k := range parsedTemplates {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}
