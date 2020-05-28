package argmapper

import "reflect"

// Convert converts the input arguments to the given target type. Convert will
// use any of the available arguments and converters to reach the given target
// type.
func Convert(target reflect.Type, opts ...Arg) (interface{}, error) {
	out, err := convertMulti([]reflect.Type{target}, opts...)
	if err != nil {
		return nil, err
	}

	return out[0].Interface(), nil
}

// convertMulti is the same as Convert but converts multiple types at one
// time. This is not currently exposed publicly since it is a slightly more
// complicated interfaces and callers can call Convert multiple times. But
// we use this internally for Redefine and FilterOutput.
func convertMulti(target []reflect.Type, opts ...Arg) ([]reflect.Value, error) {
	// The way we get convert to work is that we make a dynamic function
	// that takes the target type as input, and then call Call on it. This
	// lets our DI system automatically determine our conversion.
	f, err := convertFunc(target)
	if err != nil {
		return nil, err
	}

	// Call it
	result := f.Call(opts...)
	if err := result.Err(); err != nil {
		return nil, err
	}

	// Our result is the first result
	return result.out, nil
}

// convertFunc constructs a function that takes values of the given target
// and returns them directly. We use this for conversion.
func convertFunc(target []reflect.Type) (*Func, error) {
	funcType := reflect.FuncOf(target, target, false)
	funcVal := reflect.MakeFunc(funcType, func(args []reflect.Value) []reflect.Value {
		return args
	})

	// Create our "Func" type from our newly created function.
	return NewFunc(funcVal.Interface())
}
