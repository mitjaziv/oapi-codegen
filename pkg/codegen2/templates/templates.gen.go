package templates

import "text/template"

var templates = map[string]string{"param-type.tmpl": `// Package {{ .PackageName }} provides primitives to interact the openapi HTTP API.
//
// Code generated by github.com/deepmap/oapi-codegen DO NOT EDIT.
package {{ .PackageName }}

{{ if .Imports }}
    import (
    {{ range .Imports }} {{ . }}
    {{ end }})
{{ end }}

{{ $opid := .Operation.OperationId }}
{{ range .Operation.TypeDefinitions }}
// {{ .TypeName }} defines parameters for {{ $opid }}.
type {{ .TypeName }} {{ .Schema.TypeDecl }}
{{ end }}
`,
	"request-bodies.tmpl": `{{ range . }}{{ $opid := .OperationId }}
{{ range .Bodies }}
// {{ $opid }}RequestBody defines body for {{ $opid }} for application/json ContentType.
type {{ $opid }}{{ .NameTag }}RequestBody {{ .TypeDef }}
{{ end }}
{{ end }}
`,
	"type-properties.tmpl": `{{ $addType := .Type.Schema.AdditionalPropertiesType.TypeDecl }}

// Getter for additional properties for {{ .Type.TypeName }}. Returns the specified
// element and whether it was found
func (a {{ .Type.TypeName }}) Get(fieldName string) (value {{ $addType }}, found bool) {
    if a.AdditionalProperties != nil {
        value, found = a.AdditionalProperties[fieldName]
    }
    return
}

// Setter for additional properties for {{ .Type.TypeName }}
func (a *{{ .Type.TypeName }}) Set(fieldName string, value {{ $addType }}) {
    if a.AdditionalProperties == nil {
        a.AdditionalProperties = make(map[string]{{ $addType }})
    }
    a.AdditionalProperties[fieldName] = value
}

// Override default JSON handling for {{ .Type.TypeName }} to handle AdditionalProperties
func (a *{{ .Type.TypeName }}) UnmarshalJSON(b []byte) error {
    object := make(map[string]json.RawMessage)
	err := json.Unmarshal(b, &object)
	if err != nil {
		return err
	}
{{ range .Type.Schema.Properties }}
    if raw, found := object["{{ .Type.JsonFieldName }}"]; found {
        err = json.Unmarshal(raw, &a.{{ .Type.GoFieldName }})
        if err != nil {
            return errors.Wrap(err, "error reading '{{ .Type.JsonFieldName }}'")
        }
        delete(object, "{{ .Type.JsonFieldName }}")
    }
{{ end }}
    if len(object) != 0 {
        a.AdditionalProperties = make(map[string]{{ $addType }})
        for fieldName, fieldBuf := range object {
            var fieldVal {{ $addType }}
            err := json.Unmarshal(fieldBuf, &fieldVal)
            if err != nil {
                return errors.Wrap(err, fmt.Sprintf("error unmarshaling field %s", fieldName))
            }
            a.AdditionalProperties[fieldName] = fieldVal
        }
    }
	return nil
}

// Override default JSON handling for {{ .Type.TypeName }} to handle AdditionalProperties
func (a {{ .Type.TypeName }}) MarshalJSON() ([]byte, error) {
    var err error
    object := make(map[string]json.RawMessage)
{{ range .Type.Schema.Properties }}
{{ if not .Required }}if a.{{ .Type.GoFieldName }} != nil { {{ end }}
    object["{{ .Type.JsonFieldName }}"], err = json.Marshal(a.{{ .Type.GoFieldName }})
    if err != nil {
        return nil, errors.Wrap(err, fmt.Sprintf("error marshaling '{{ .Type.JsonFieldName }}'"))
    }
{{ if not .Type.Required}} }{{ end }}
{{ end }}
    for fieldName, field := range a.AdditionalProperties {
		object[fieldName], err = json.Marshal(field)
		if err != nil {
			return nil, errors.Wrap(err, fmt.Sprintf("error marshaling '%s'", fieldName))
		}
	}
	return json.Marshal(object)
}
`,
	"type.tmpl": `// Package {{.PackageName}} provides primitives to interact the openapi HTTP API.
//
// Code generated by github.com/deepmap/oapi-codegen DO NOT EDIT.
package {{.PackageName}}

{{if .Imports}}
    import (
    {{range .Imports}} {{ . }}
    {{end}})
{{end}}

// {{.Type.TypeName}} defines model for {{.Type.JsonName}}.
type {{.Type.TypeName}} {{.Type.Schema.TypeDecl}}
`,
}

// Parse parses declared templates.
func Parse(t *template.Template) (*template.Template, error) {
	for name, s := range templates {
		var tmpl *template.Template
		if t == nil {
			t = template.New(name)
		}
		if name == t.Name() {
			tmpl = t
		} else {
			tmpl = t.New(name)
		}
		if _, err := tmpl.Parse(s); err != nil {
			return nil, err
		}
	}
	return t, nil
}

