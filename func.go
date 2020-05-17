package argmapper

import (
	"fmt"
	"reflect"
)

type Func struct {
	fn    reflect.Value
	input *structType
}

func NewFunc(f interface{}) (*Func, error) {
	fv := reflect.ValueOf(f)
	ft := fv.Type()
	if k := ft.Kind(); k != reflect.Func {
		return nil, fmt.Errorf("fn should be a function, got %s", k)
	}

	// We only accept zero or 1 arguments right now. In the future we
	// could potentially expand this to support multiple args that are
	// all structs we populate but for now lets just simplify this.
	if ft.NumIn() > 1 {
		return nil, fmt.Errorf("function must take one struct arg")
	}

	// Our argument must be a struct
	typ := ft.In(0)
	if typ.Kind() != reflect.Struct {
		return nil, fmt.Errorf("function must take one struct arg")
	}

	structTyp, err := newStructType(typ)
	if err != nil {
		return nil, err
	}

	return &Func{
		fn:    fv,
		input: structTyp,
	}, nil
}

func (f *Func) String() string {
	return f.fn.String()
}

// errType is used for comparison in Spec
var errType = reflect.TypeOf((*error)(nil)).Elem()
