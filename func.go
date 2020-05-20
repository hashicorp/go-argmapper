package argmapper

import (
	"fmt"
	"reflect"
)

type Func struct {
	fn    reflect.Value
	input *valueSet
}

func NewFunc(f interface{}) (*Func, error) {
	fv := reflect.ValueOf(f)
	ft := fv.Type()
	if k := ft.Kind(); k != reflect.Func {
		return nil, fmt.Errorf("fn should be a function, got %s", k)
	}

	structTyp, err := newValueSet(ft.NumIn(), ft.In)
	if err != nil {
		return nil, err
	}

	return &Func{
		fn:    fv,
		input: structTyp,
	}, nil
}

// errType is used for comparison in Spec
var errType = reflect.TypeOf((*error)(nil)).Elem()
