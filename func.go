package argmapper

import (
	"fmt"
	"reflect"
	"unicode"
	"unicode/utf8"

	"github.com/hashicorp/go-multierror"
)

// Result is returned from a Call with the results of the function call.
// This structure lets you access multiple results.
type Result struct {
	out      []reflect.Value
	buildErr error
}

// Err returns any error that occurred as part of the call. This can
// be an error in the process of calling or it can be an error from the
// result of the call. argmapper automatically detects a non-nil final
// output as an error.
func (r *Result) Err() error {
	if r.buildErr != nil {
		return r.buildErr
	}

	if len(r.out) > 0 {
		final := r.out[len(r.out)-1]
		if final.Type() == errType {
			return final.Interface().(error)
		}

		return nil
	}

	return nil
}

// Out returns the i'th result (zero-indexed) of the function. This will
// panic if i >= Len so for safety all calls to Out should check Len.
func (r *Result) Out(i int) interface{} {
	return r.out[i].Interface()
}

// Len returns the number of outputs.
func (r *Result) Len() int {
	return len(r.out)
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

	return &Func{
		fn:  fv,
		arg: typ,
	}, nil
}

type Func struct {
	fn  reflect.Value
	arg reflect.Type
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

	// We want to populate our args to instantiate our struct
	in := reflect.New(f.arg).Elem()
	for i := 0; i < f.arg.NumField(); i++ {
		structField := f.arg.Field(i)

		// Ignore unexported fields
		if structField.PkgPath != "" {
			continue
		}

		value, ok := builder.named[firstToLower(structField.Name)]
		if !ok {
			resultErr = multierror.Append(resultErr, fmt.Errorf(
				"argument cannot be satisfied: %s", structField.Name))
			continue
		}

		in.Field(i).Set(reflect.ValueOf(value))
	}

	// If we have an error, then we don't even call we just set it and return
	if resultErr != nil {
		return Result{buildErr: resultErr}
	}

	// Call our function
	out := f.fn.Call([]reflect.Value{in})

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
