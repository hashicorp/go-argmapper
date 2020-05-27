package argmapper

import "reflect"

// Convert converts the input arguments to the given target type. Convert will
// use any of the available arguments and converters to reach the given target
// type.
func Convert(target reflect.Type, opts ...Arg) (interface{}, error) {
	// The way we get convert to work is that we make a dynamic function
	// that takes the target type as input, and then call Call on it. This
	// lets our DI system automatically determine our conversion.
	funcType := reflect.FuncOf([]reflect.Type{target}, []reflect.Type{target}, false)
	funcVal := reflect.MakeFunc(funcType, func(args []reflect.Value) []reflect.Value {
		return args
	})

	// Create our "Func" type from our newly created function.
	f, err := NewFunc(funcVal.Interface())
	if err != nil {
		return nil, err
	}

	// Call it
	result := f.Call(opts...)
	if err := result.Err(); err != nil {
		return nil, err
	}

	// Our result is the first result
	return result.Out(0), nil
}
