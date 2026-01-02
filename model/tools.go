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
	typ         reflect.Type
}

type ToolArg struct {
	Arg         string
	Description string
	Optional    bool
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

func (tls Tools) Call(name string, args json.RawMessage) (string, error) {
	for _, tl := range tls {
		if tl.Name == name {
			return tl.Call(args)
		}
	}

	return "", fmt.Errorf("function not found: %s", name)
}

var (
	errorType = reflect.TypeOf((*error)(nil)).Elem()
)

func (tl *Tool) Build() error {
	if tl.typ != nil {
		return nil
	}

	typ := reflect.TypeOf(tl.Func)
	if typ.Kind() != reflect.Func {
		return fmt.Errorf("expected a function: %T", tl.Func)
	}

	numArgs := typ.NumIn()
	if numArgs != len(tl.Args) {
		return errors.New("args must match function arguments")
	}

	// XXX: build up the parameters based on the typ
	for i := 0; i < numArgs; i += 1 {
		fmt.Println("in:", i, typ.In(i).Name())
	}

	if typ.NumOut() != 2 || typ.Out(0).Kind() != reflect.String ||
		!typ.Out(1).Implements(errorType) {

		return errors.New("expected a function that returns (string, error)")
	}

	tl.typ = typ
	return nil
}

func (tl *Tool) Call(args json.RawMessage) (string, error) {
	if tl.typ == nil {
		panic(fmt.Sprintf("tool %s must be built before use", tl.Name))
	}

	// XXX
	return "", nil
}
