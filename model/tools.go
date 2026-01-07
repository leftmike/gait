package model

import (
	"fmt"
	"reflect"
	"strings"
	"unicode"
)

type Tools []Tool

type ToolFunc func(buf []byte) (string, error)

type Tool struct {
	Name        string
	Description string
	Func        ToolFunc
	Schema      *ToolSchema
}

type ToolSchema struct {
	schema map[string]any
	typ    reflect.Type
}

func fieldNameToJSON(s string) string {
	rs := []rune(s)
	nam := make([]rune, 0, len(rs))
	for i, r := range rs {
		if unicode.IsUpper(r) {
			if i > 0 && (unicode.IsLower(rs[i-1]) || (i < len(rs)-1 && unicode.IsLower(rs[i+1]))) {
				nam = append(nam, '_')
			}
			nam = append(nam, unicode.ToLower(r))
		} else {
			nam = append(nam, r)
		}
	}

	return string(nam)
}

func structToSchema(typ reflect.Type, mayBeNull bool) (map[string]any, error) {
	var req []any
	props := map[string]any{}
	for i := 0; i < typ.NumField(); i += 1 {
		var name, desc string
		var optional bool

		fld := typ.Field(i)
		jt, ok := fld.Tag.Lookup("json")
		if ok {
			vals := strings.Split(jt, ",")
			name = vals[0]
			for j := 1; j < len(vals); j += 1 {
				if vals[j] == "omitzero" || vals[j] == "omitempty" {
					optional = true
				}
			}
		}
		gt, ok := fld.Tag.Lookup("gait")
		if ok {
			if vals := strings.Split(gt, ","); len(vals) > 1 {
				return nil, fmt.Errorf("only one value allowed for gait tag: %s: %s",
					fld.Name, gt)
			}
			desc = gt
		}

		ftyp := fld.Type
		var pointer bool
		if ftyp.Kind() == reflect.Pointer {
			pointer = true
			ftyp = ftyp.Elem()
		}

		if name == "-" {
			continue
		} else if name == "" {
			name = fieldNameToJSON(fld.Name)
		}

		fscm, err := typeToSchema(ftyp, pointer)
		if err != nil {
			return nil, err
		}
		if desc != "" {
			fscm["description"] = desc
		}
		props[name] = fscm

		if !optional {
			req = append(req, name)
		}
	}

	scm := map[string]any{
		"properties": props,
	}
	if len(req) > 0 {
		scm["required"] = req
	}
	if mayBeNull {
		scm["type"] = []any{"null", "object"}
	} else {
		scm["type"] = "object"
	}
	return scm, nil
}

var (
	kindToSimpleJSONType = map[reflect.Kind]string{
		reflect.Bool:    "boolean",
		reflect.Int:     "integer",
		reflect.Int8:    "integer",
		reflect.Int16:   "integer",
		reflect.Int32:   "integer",
		reflect.Int64:   "integer",
		reflect.Uint:    "integer",
		reflect.Uint8:   "integer",
		reflect.Uint16:  "integer",
		reflect.Uint32:  "integer",
		reflect.Uint64:  "integer",
		reflect.Float32: "number",
		reflect.Float64: "number",
		reflect.String:  "string",
	}

	formats = map[reflect.Kind]string{
		reflect.Int32:   "int32",
		reflect.Int64:   "int64",
		reflect.Float32: "float",
		reflect.Float64: "double",
	}
)

func typeToSchema(typ reflect.Type, mayBeNull bool) (map[string]any, error) {
	kind := typ.Kind()
	switch kind {
	case reflect.Struct:
		return structToSchema(typ, mayBeNull)

	case reflect.Slice:
		items, err := typeToSchema(typ.Elem(), false)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"type":  []interface{}{"null", "array"},
			"items": items,
		}, nil

	case reflect.Array:
		items, err := typeToSchema(typ.Elem(), false)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"type":     "array",
			"maxItems": float64(typ.Len()),
			"minItems": float64(typ.Len()),
			"items":    items,
		}, nil

	case reflect.Map:
		if typ.Key().Kind() != reflect.String {
			return nil, fmt.Errorf("maps must be string-keyed: %s", typ)
		}
		vals, err := typeToSchema(typ.Elem(), false)
		if err != nil {
			return nil, err
		}

		return map[string]any{
			"type":                 "object",
			"additionalProperties": vals,
		}, nil

	default:
		jsonType, ok := kindToSimpleJSONType[kind]
		if !ok {
			return nil, fmt.Errorf("type not supported: %s", typ)
		}
		scm := map[string]any{}
		if mayBeNull {
			scm["type"] = []any{"null", jsonType}
		} else {
			scm["type"] = jsonType
		}
		format, ok := formats[kind]
		if ok {
			scm["format"] = format
		}

		return scm, nil
	}
}

func NewToolSchema[T any]() (*ToolSchema, error) {
	typ := reflect.TypeFor[T]()
	if typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct {
		return nil, fmt.Errorf("tool arguments must be a (pointer to a) struct: %s", typ)
	}

	scm, err := structToSchema(typ, false)
	if err != nil {
		return nil, err
	}
	return &ToolSchema{
		schema: scm,
		typ:    typ,
	}, nil
}

func MustToolSchema[T any]() *ToolSchema {
	ts, err := NewToolSchema[T]()
	if err != nil {
		var v T
		panic(fmt.Sprintf("new tool schema failed: %T: %s", v, err))
	}
	return ts
}

func (tls Tools) Call(name string, args []byte, opts *Options) (string, error) {
	for _, tl := range tls {
		if tl.Name == name {
			return tl.Func(args)
		}
	}

	return "", fmt.Errorf("function not found: %s", name)
}
