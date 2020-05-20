package argmapper

import (
	"reflect"
	"strings"

	"github.com/hashicorp/go-hclog"
)

// Arg is an option to Func.Call that sets the state for the function call.
// This can be a direct named arg or a converter that could be used if
// necessary to reach the target.
type Arg func(*argBuilder) error

type argBuilder struct {
	logger hclog.Logger
	named  map[string]reflect.Value
	convs  []*Func
}

// Named specifies a named argument with the given value. This will satisfy
// any requirement where the name matches AND the value is assignable to
// the struct.
func Named(n string, v interface{}) Arg {
	return func(a *argBuilder) error {
		a.named[strings.ToLower(n)] = reflect.ValueOf(v)
		return nil
	}
}

// WithConvFunc specifies one or more converters to use if necessary.
// A converter will be used if an argument type doesn't match exactly.
func WithConvFunc(fs ...interface{}) Arg {
	return func(a *argBuilder) error {
		for _, f := range fs {
			conv, err := NewFunc(f)
			if err != nil {
				return err
			}

			a.convs = append(a.convs, conv)
		}

		return nil
	}
}
