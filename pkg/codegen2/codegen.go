package codegen2

import (
	"bufio"
	"bytes"
	"fmt"
	"go/format"
	"os"
	"regexp"
	"strings"
	"text/template"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/pkg/errors"

	"github.com/deepmap/oapi-codegen/pkg/codegen"
	"github.com/deepmap/oapi-codegen/pkg/codegen2/templates"
)

func Generate(swagger *openapi3.Swagger, opts codegen.Options) error {
	if opts.TargetDir != "" {
		if _, err := os.Stat(opts.TargetDir); os.IsNotExist(err) {
			os.Mkdir(opts.TargetDir, 0777)
		}
	}

	// This creates the golang templates text package
	t := template.New("oapi-codegen").Funcs(codegen.TemplateFunctions)
	// This parses all of our own template files into the template object
	// above
	t, err := templates.Parse(t)
	if err != nil {
		return errors.Wrap(err, "error parsing oapi-codegen templates")
	}

	// Override built-in templates with user-provided versions
	for _, tpl := range t.Templates() {
		if _, ok := opts.UserTemplates[tpl.Name()]; ok {
			utpl := t.New(tpl.Name())
			_, err := utpl.Parse(opts.UserTemplates[tpl.Name()])
			if err != nil {
				return errors.Wrapf(err, "error parsing user-provided template %q", tpl.Name())
			}
		}
	}

	// Generate operations from swagger specification.
	operations, err := codegen.GenerateOperations(swagger)
	if err != nil {
		return errors.Wrap(err, "error creating operation definitions")
	}

	// Generate types if enabled.
	if opts.GenerateTypes {
		err := GenerateTypeDefinitions(t, swagger, opts, operations)
		if err != nil {
			return errors.Wrap(err, "error generating type definitions")
		}
	}
	return nil
}

func GenerateTypeDefinitions(t *template.Template, swagger *openapi3.Swagger, opts codegen.Options, ops codegen.Operations) error {
	if _, err := os.Stat(opts.TargetDir + "types/"); os.IsNotExist(err) {
		os.Mkdir(opts.TargetDir+"types/", 0777)
	}

	schemaTypes, err := codegen.GenerateTypesForSchemas(t, swagger.Components.Schemas)
	if err != nil {
		return errors.Wrap(err, "error generating Go types for component schemas")
	}

	paramTypes, err := codegen.GenerateTypesForParameters(t, swagger.Components.Parameters)
	if err != nil {
		return errors.Wrap(err, "error generating Go types for component parameters")
	}
	allTypes := append(schemaTypes, paramTypes...)

	responseTypes, err := codegen.GenerateTypesForResponses(t, swagger.Components.Responses)
	if err != nil {
		return errors.Wrap(err, "error generating Go types for component responses")
	}
	allTypes = append(allTypes, responseTypes...)

	bodyTypes, err := codegen.GenerateTypesForRequestBodies(t, swagger.Components.RequestBodies)
	if err != nil {
		return errors.Wrap(err, "error generating Go types for component request bodies")
	}
	allTypes = append(allTypes, bodyTypes...)

	// Generate files for types.
	err = GenerateTypes(t, opts, allTypes)
	if err != nil {
		return errors.Wrap(err, "error generating code for type definitions")
	}

	// Generate types for operations.
	err = GenerateTypesForOperations(t, opts, ops.Definitions)
	if err != nil {
		return errors.Wrap(err, "error generating Go types for operation parameters")
	}

	return nil
}

// Helper function to pass a bunch of types to the template engine, and buffer
// its output into a string.
func GenerateTypes(t *template.Template, opts codegen.Options, types []codegen.TypeDefinition) error {
	data := struct {
		PackageName string
		Imports     []string
		Type        codegen.TypeDefinition
	}{
		PackageName: opts.PackageName,
		Imports:     []string{},
		Type:        codegen.TypeDefinition{},
	}

	for _, typ := range types {
		// Create filename.
		filename := opts.TargetDir + "types/" + typ.TypeName + ".gen.go"

		// Generate imports
		var imports []string
		for _, goImport := range codegen.AllGoImports {
			match, err := regexp.MatchString(fmt.Sprintf("[^a-zA-Z0-9_]%s", goImport.LookFor), typ.Schema.TypeDecl())
			if err != nil {
				return errors.Wrap(err, "error figuring out imports")
			}
			if match {
				imports = append(imports, goImport.String())
			}
		}

		// Populate data
		data.Imports = imports
		data.Type = typ

		// Generate type definition code from template.
		var tpl bytes.Buffer
		err := t.ExecuteTemplate(&tpl, "type.tmpl", data)
		if err != nil {
			return errors.Wrap(err, "error generating type definition code")
		}
		code := tpl.String()

		// Generate additional property code.
		var properties string
		if typ.Schema.HasAdditionalProperties {
			var tpl bytes.Buffer
			err := t.ExecuteTemplate(&tpl, "type-properties.tmpl", data)
			if err != nil {
				return errors.Wrap(err, "error generating additional properties code")
			}

			properties += tpl.String()
		}

		// Joins type code with parameters code.
		code = strings.Join([]string{code, properties}, "\n\n")

		// Write code to file.
		err = writeCodeToFile(filename, code)
		if err != nil {
			return errors.Wrap(err, "error writing type definition to file")
		}
	}
	return nil
}

// Generates code for all types produced
func GenerateTypesForOperations(t *template.Template, opts codegen.Options, ops []codegen.OperationDefinition) error {
	data := struct {
		PackageName string
		Imports     []string
		Operation   codegen.OperationDefinition
	}{
		PackageName: opts.PackageName,
		Imports:     []string{},
		Operation:   codegen.OperationDefinition{},
	}

	for _, op := range ops {
		// Skip operation if it does not have any Type Definitions.
		if len(op.TypeDefinitions) == 0 {
			continue
		}

		// Create filename.
		filename := opts.TargetDir + "types/" + op.OperationId + ".gen.go"

		// Generate imports and additional properties code.
		var imports []string
		var properties string
		for _, typ := range op.TypeDefinitions {

			// Generate imports
			for _, goImport := range codegen.AllGoImports {
				match, err := regexp.MatchString(fmt.Sprintf("[^a-zA-Z0-9_]%s", goImport.LookFor), typ.Schema.TypeDecl())
				if err != nil {
					return errors.Wrap(err, "error figuring out imports")
				}
				if match {
					imports = append(imports, goImport.String())
				}
			}

			// Generate additional property code.
			if typ.Schema.HasAdditionalProperties {
				data := struct {
					PackageName string
					Type        codegen.TypeDefinition
				}{
					PackageName: opts.PackageName,
					Type:        typ,
				}

				var tpl bytes.Buffer
				err := t.ExecuteTemplate(&tpl, "type-properties.tmpl", data)
				if err != nil {
					return errors.Wrap(err, "error generating additional properties code")
				}
				properties = tpl.String()
			}
		}

		// Generate request bodies
		var tpl bytes.Buffer
		err := t.ExecuteTemplate(&tpl, "request-bodies.tmpl", ops)
		if err != nil {
			return errors.Wrap(err, "error generating request bodies for operations")
		}
		bodies := tpl.String()

		// Fill data
		data.Imports = imports
		data.Operation = op

		// Generate parameter types code from template.
		tpl.Reset()
		err = t.ExecuteTemplate(&tpl, "param-type.tmpl", data)
		if err != nil {
			return errors.Wrap(err, "error generating type definition code")
		}

		// Joins type code with parameters code.
		code := strings.Join([]string{tpl.String(), properties, bodies}, "\n\n")

		// Write code to file.
		err = writeCodeToFile(filename, code)
		if err != nil {
			return errors.Wrap(err, "error writing type definition to file")
		}
	}
	return nil
}

func writeCodeToFile(filename string, code string) error {
	// SanitizeCode code
	code = codegen.SanitizeCode(code)

	// Format code.
	bytes, err := format.Source([]byte(code))
	if err != nil {
		return errors.Wrap(err, "error formatting code")
	}

	// Create file.
	f, err := os.Create(filename)
	if err != nil {
		return errors.Wrap(err, "error creating file")
	}
	defer f.Close()

	// Write code to file.
	w := bufio.NewWriter(f)

	_, err = w.Write(bytes)
	if err != nil {
		return errors.Wrap(err, "error writing to file")
	}
	err = w.Flush()
	if err != nil {
		return errors.Wrap(err, "error flushing writer")
	}
	return nil
}
