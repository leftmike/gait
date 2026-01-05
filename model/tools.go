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

var (
	kindToJSONType = map[reflect.Kind]string{
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
)

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

func NewToolSchema[T any]() (*ToolSchema, error) {
	typ := reflect.TypeFor[T]()
	if typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct {
		return nil, fmt.Errorf("tool arguments must be a (pointer to a) struct: %s", typ)
	}

	var req []string
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
		if ftyp.Kind() == reflect.Pointer {
			optional = true
			ftyp = ftyp.Elem()
		}

		if name == "" {
			name = fieldNameToJSON(fld.Name)
		}
		if desc == "" {
			desc = name
		}

		// XXX: struct, array, slice; fail on bad types
		props[name] = map[string]any{
			"type":        kindToJSONType[ftyp.Kind()],
			"description": desc,
		}

		if !optional {
			req = append(req, name)
		}
	}

	scm := map[string]any{
		"type":       "object",
		"properties": props,
	}
	if len(req) > 0 {
		scm["required"] = req
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

/*
type ToolArg struct {
	Name        string
	Description string
	Optional    bool
	typ         reflect.Type
}
*/

func (tls Tools) Call(name string, args []byte, opts *Options) (string, error) {
	for _, tl := range tls {
		if tl.Name == name {
			return tl.Func(args)
		}
	}

	return "", fmt.Errorf("function not found: %s", name)
}

/*
var (
	errorType = reflect.TypeOf((*error)(nil)).Elem()

	invalidKind = [reflect.UnsafePointer + 1]bool{
		reflect.Invalid:       true,
		reflect.Uintptr:       true,
		reflect.Complex64:     true,
		reflect.Complex128:    true,
		reflect.Array:         true,
		reflect.Chan:          true,
		reflect.Func:          true,
		reflect.Interface:     true,
		reflect.Map:           true,
		reflect.Pointer:       true,
		reflect.Slice:         true,
		reflect.Struct:        true,
		reflect.UnsafePointer: true,
	}
)

func (tl *Tool) Build() error {
	if tl.built {
		return nil
	}

	typ := reflect.TypeOf(tl.Func)
	if typ.Kind() != reflect.Func {
		return fmt.Errorf("expected a function: %s %T", tl.Name, tl.Func)
	}

	if typ.IsVariadic() {
		return fmt.Errorf("function must not be variadic: %s", tl.Name)
	}

	numArgs := typ.NumIn()
	if numArgs != len(tl.Args) {
		return fmt.Errorf("args must match function arguments: %s", tl.Name)
	}

	for i := 0; i < numArgs; i += 1 {
		atyp := typ.In(i)
		kind := atyp.Kind()
		if invalidKind[kind] {
			return fmt.Errorf("invalid parameter type: %s %s", tl.Name, kind)
		}
		tl.Args[i].typ = atyp
	}

	if typ.NumOut() != 2 || typ.Out(0).Kind() != reflect.String ||
		!typ.Out(1).Implements(errorType) {

		return errors.New("expected a function that returns (string, error)")
	}

	tl.val = reflect.ValueOf(tl.Func)
	tl.built = true
	return nil
}

func (tl *Tool) Call(buf []byte, opts *Options) (string, error) {
	if !tl.built {
		panic(fmt.Sprintf("tool must be built before use: %s", tl.Name))
	}

	var jsonArgs map[string]json.RawMessage
	err := json.Unmarshal(buf, &jsonArgs)
	if err != nil {
		return "", err
	}

	args := make([]reflect.Value, len(tl.Args))
	for i, arg := range tl.Args {
		buf, ok := jsonArgs[arg.Name]
		if !ok {
			if !arg.Optional {
				return "", fmt.Errorf("missing required argument: %s", arg.Name)
			}
			continue
		}

		val := reflect.New(arg.typ)
		err := json.Unmarshal(buf, val.Interface())
		if err != nil {
			return "", err
		}
		args[i] = val.Elem()
	}

	ret := tl.val.Call(args)
	if len(ret) != 2 || !ret[1].Type().Implements(errorType) {
		panic(fmt.Sprintf("unexpected: %s should return (string, error)", tl.Name))
	}

	if !ret[1].IsNil() {
		return "", ret[1].Interface().(error)
	}
	return ret[0].Interface().(string), nil
}
*/
