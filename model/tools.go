package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
)

type Tools []*Tool

type Tool struct {
	Name        string
	Description string
	Args        []ToolArg
	Func        any
	val         reflect.Value
	built       bool
}

type ToolArg struct {
	Name        string
	Description string
	Optional    bool
	typ         reflect.Type
}

func (tls Tools) Build() error {
	for _, tl := range tls {
		err := tl.Build()
		if err != nil {
			return err
		}
	}

	return nil
}

func (tls Tools) Call(name string, args []byte, opts *Options) (string, error) {
	for _, tl := range tls {
		if tl.Name == name {
			return tl.Call(args, opts)
		}
	}

	return "", fmt.Errorf("function not found: %s", name)
}

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

/*
Consider using https://github.com/google/jsonschema-go
https://pkg.go.dev/github.com/google/jsonschema-go@v0.4.2/jsonschema#pkg-overview
*/

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
