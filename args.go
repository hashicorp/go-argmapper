package argmapper

import (
	"reflect"
	"strings"
)

type Arg func(*argBuilder) error

type argBuilder struct {
	named map[string]reflect.Value
	convs []*Conv
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
			conv, err := NewConv(f)
			if err != nil {
				return err
			}

			a.convs = append(a.convs, conv)
		}

		return nil
	}
}

// withNamedReflectValues sets the map of known values directly. We use
// this internally for calling converters. This is not exported since its
// not a public API we want to support currently and it can be simulated
// with repeated calls to Named.
func withNamedReflectValues(v map[string]reflect.Value) Arg {
	return func(a *argBuilder) error {
		a.named = v
		return nil
	}
}
