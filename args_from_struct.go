package argmapper

import (
	"fmt"
	"reflect"
)

type FieldArg func(reflect.StructField, reflect.Value) []Arg

// TODO: something to flatten embedded (but propogate args)
// func IncludeEmbeddedFields(opts ...FieldArg) FieldArg {
// 	return func(f reflect.StructField, v reflect.Value) []Arg {
// 		var args []Arg

// 		if fst := structValueOf(v); f.Anonymous && fst.Kind() != reflect.Invalid {
// 			args = append(args, FromStruct(v.Interface(), opts...)...)
// 		}

// 		return args
// 	}
// }

func TypedFields() FieldArg {
	return func(f reflect.StructField, v reflect.Value) []Arg {
		args := []Arg{
			Typed(v.Interface()),
		}

		return args
	}
}

func NamedFields() FieldArg {
	// TODO: only exported?
	return func(f reflect.StructField, v reflect.Value) []Arg {
		args := []Arg{
			Named(f.Name, v.Interface()),
		}

		return args
	}
}

func FromStruct(v interface{}, opts ...FieldArg) []Arg {
	rv := reflect.ValueOf(v)
	sv := structValueOf(rv)
	if sv.Kind() == reflect.Invalid {
		panic(fmt.Sprintf("only struct or pointer to struct types are supported in FromStruct, got %T", v))
	}
	st := sv.Type()

	var args []Arg
	for i := 0; i < st.NumField(); i++ {
		f := st.Field(i)
		fv := sv.Field(i)
		for _, opt := range opts {
			args = append(args, opt(f, fv)...)
		}
	}

	return args
}

func structValueOf(rv reflect.Value) reflect.Value {
	if k := rv.Kind(); k != reflect.Struct && k != reflect.Ptr {
		return reflect.Value{}
	}

	sv := rv
	if sv.Kind() == reflect.Ptr {
		// unwrap ptr
		sv = sv.Elem()
		if sv.Kind() != reflect.Struct {
			return reflect.Value{}
		}
	}

	return sv
}
