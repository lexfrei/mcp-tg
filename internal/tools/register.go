package tools

import (
	"reflect"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// BoolFieldRegistry mirrors middleware.BoolFieldRegistry without creating a
// dependency on the middleware package. The cmd/mcp-tg entry point passes
// the same map to both packages.
type BoolFieldRegistry = map[string]map[string]struct{}

// AddTool wraps mcp.AddTool, additionally recording the JSON names of every
// bool / *bool field on the In type into registry under the tool's name. The
// registry is later used by the bool-coercion middleware to rewrite
// string-encoded booleans (e.g. "true") into real JSON booleans before the
// SDK validator runs.
func AddTool[In, Out any](
	server *mcp.Server,
	registry BoolFieldRegistry,
	tool *mcp.Tool,
	handler mcp.ToolHandlerFor[In, Out],
) {
	fields := boolJSONFields[In]()
	if len(fields) > 0 && registry != nil {
		registry[tool.Name] = fields
	}

	mcp.AddTool(server, tool, handler)
}

// boolJSONFields returns the JSON field names of every bool/*bool member of
// the struct type T. Empty map when there are none.
func boolJSONFields[T any]() map[string]struct{} {
	var zero T

	return collectBoolJSONFields(reflect.TypeOf(zero))
}

func collectBoolJSONFields(typ reflect.Type) map[string]struct{} {
	if typ == nil {
		return nil
	}

	if typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}

	if typ.Kind() != reflect.Struct {
		return nil
	}

	out := make(map[string]struct{})

	for field := range typ.Fields() {
		ft := field.Type
		if ft.Kind() == reflect.Pointer {
			ft = ft.Elem()
		}

		if ft.Kind() != reflect.Bool {
			continue
		}

		name := jsonFieldName(&field)
		if name != "" {
			out[name] = struct{}{}
		}
	}

	if len(out) == 0 {
		return nil
	}

	return out
}

func jsonFieldName(field *reflect.StructField) string {
	tag, ok := field.Tag.Lookup("json")
	if !ok {
		return field.Name
	}

	if comma := strings.IndexByte(tag, ','); comma >= 0 {
		tag = tag[:comma]
	}

	if tag == "-" || tag == "" {
		return ""
	}

	return tag
}
