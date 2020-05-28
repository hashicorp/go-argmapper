package argmapper

import "reflect"

// Result is returned from a Call with the results of the function call.
// This structure lets you access multiple results.
type Result struct {
	*ValueSet

	out      []reflect.Value
	buildErr error
}

// resultError returns a Result with an error.
func resultError(err error) Result {
	return Result{buildErr: err}
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
		if final.IsValid() && final.Type() == errType {
			if err := final.Interface(); err != nil {
				return err.(error)
			}
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

// Len returns the number of outputs, excluding any final error output.
func (r *Result) Len() int {
	result := len(r.out)
	if r.hasError() {
		result -= 1
	}

	return result
}

func (r *Result) hasError() bool {
	if len(r.out) == 0 {
		return false
	}

	final := r.out[len(r.out)-1]
	return final.Type() == errType
}
