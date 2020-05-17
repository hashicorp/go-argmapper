package argmapper

import (
	"reflect"
	"strings"
)

type Arg func(*callState) error

// Named specifies a named argument with the given value. This will satisfy
// any requirement where the name matches AND the value is assignable to
// the struct.
func Named(n string, v interface{}) Arg {
	return func(s *callState) error {
		s.Named[strings.ToLower(n)] = reflect.ValueOf(v)
		return nil
	}
}

// WithConvFunc specifies one or more converters to use if necessary.
// A converter will be used if an argument type doesn't match exactly.
func WithConvFunc(fs ...interface{}) Arg {
	return func(s *callState) error {
		for _, f := range fs {
			conv, err := NewConv(f)
			if err != nil {
				return err
			}

			s.Convs = append(s.Convs, conv)
		}

		return nil
	}
}
