package argmapper

import (
	"fmt"
	"reflect"
	"unicode"
	"unicode/utf8"

	"github.com/hashicorp/go-multierror"
)

type Func struct {
	fn  reflect.Value
	arg *structType
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
		fn:  fv,
		arg: structTyp,
	}, nil
}

func (f *Func) Call(opts ...Arg) Result {
	// Build up our args
	builder := &argBuilder{
		named: make(map[string]interface{}),
	}
	for _, opt := range opts {
		opt(builder)
	}

	// resultErr builds up an error we set on Result prior to the actual
	// function call if there are errors preparing the arguments.
	var resultErr error

	// Initialize the struct we'll be populating
	structVal := f.arg.New()

	// We want to populate our args to instantiate our struct
	for k, _ := range f.arg.fields {
		value, ok := builder.named[firstToLower(k)]
		if !ok {
			resultErr = multierror.Append(resultErr, fmt.Errorf(
				"argument cannot be satisfied: %s", k))
			continue
		}

		structVal.Set(k, reflect.ValueOf(value))
	}

	// If we have an error, then we don't even call we just set it and return
	if resultErr != nil {
		return Result{buildErr: resultErr}
	}

	// Call our function
	out := f.fn.Call([]reflect.Value{structVal.Value()})

	return Result{out: out}
}

func firstToLower(s string) string {
	if len(s) > 0 {
		r, size := utf8.DecodeRuneInString(s)
		if r != utf8.RuneError || size > 1 {
			lo := unicode.ToLower(r)
			if lo != r {
				s = string(lo) + s[size:]
			}
		}
	}
	return s
}

// errType is used for comparison in Spec
var errType = reflect.TypeOf((*error)(nil)).Elem()
