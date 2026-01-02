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
	Arg         string // XXX: change to Name
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

func (tls Tools) Call(name string, args json.RawMessage, opts *Options) (string, error) {
	for _, tl := range tls {
		if tl.Name == name {
			return tl.Call(args, opts)
		}
	}

	return "", fmt.Errorf("function not found: %s", name)
}

var (
	errorType = reflect.TypeOf((*error)(nil)).Elem()
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
		// XXX: check the atyp
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

// XXX: switch to []byte for the type?
func (tl *Tool) Call(buf json.RawMessage, opts *Options) (string, error) {
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
		buf, ok := jsonArgs[arg.Arg]
		if !ok {
			if !arg.Optional {
				return "", fmt.Errorf("missing required argument: %s", arg.Arg)
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
		panic("XXX")
	}

	if !ret[1].IsNil() {
		return "", ret[1].Interface().(error)
	}
	return ret[0].Interface().(string), nil
}
