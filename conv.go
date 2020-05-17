package argmapper

import (
	"fmt"
	"reflect"

	"github.com/hashicorp/go-multierror"
)

// Conv represents a converter function that knows how to convert
// from some set of input parameters to some set of output parameters.
//
// Converters are used if a direct match argument isn't found for a Func call.
// If a converter exists (or a chain of converts) to go from the input arguments
// to the desired argument, then the chain will be called and the result used.
//
// Converter Basics
//
// Converters must take a struct as input and return a struct as output. The
// input struct is identical to a Func and arguments are mapped directly to it.
//
// The output struct is similar to the input struct, except that the keys and
// tags of the output struct will set new values for that input type. These
// values are only set for that specific chain execution. For example:
//
//    TODO
//
// Attempted Conversions
//
// The output type can also be a pointer to a struct. If a nil pointer is
// returned, the conversion is assumed to have failed. In this case, an
// alternate chain (if it exists) will be tried.
//
//    TODO
//
// Errors
//
// A second output type of error can be used to specify any errors that
// occurred during conversion. If a non-nil error is returned, alternate
// chains will be attempted. If all chains fail, the error will be reported
// to the user. In all cases, the errors are made available in the Result type
// for logging.
type Conv struct {
	*Func
	output *structType
}

// NewConv constructs a new converter. See the docs on Conv for more info.
func NewConv(f interface{}) (*Conv, error) {
	// This should be a valid function so build the function first
	fn, err := NewFunc(f)
	if err != nil {
		return nil, err
	}

	// Get our type
	ft := fn.fn.Type()

	// Validate output types.
	if ft.NumOut() != 1 {
		return nil, fmt.Errorf("function must return one or two results")
	}

	out := ft.Out(0)
	if out.Kind() != reflect.Struct {
		return nil, fmt.Errorf("first return value must be a struct or *struct")
	}

	output, err := newStructType(out)
	if err != nil {
		return nil, err
	}

	return &Conv{
		Func:   fn,
		output: output,
	}, nil
}

// outputState extracts the output from the given Result. The Result must
// be a result of calling Call on this exact Conv. Specifying any other
// Result is undefined and will likely result in panics.
func (c *Conv) outputState(r Result, state *callState) {
	// Get our struct
	v := r.out[0]

	output := c.output.inherit(r.wildcardMapping)

	// Go through the fields we know about
	for name, f := range output.fields {
		state.Named[name] = v.Field(f.Index)
	}
}

func (c *Conv) provides(f *structField) int {
	result := -1
	for _, target := range c.output.fields {
		if v := target.assignableTo(f); v > result {
			result = v
		}
	}
	for _, target := range c.output.wildcard {
		if v := target.assignableTo(f); v > result {
			result = v
		}
	}

	return result
}

// ConvSet is a set of converters.
type ConvSet []*Conv

func (s ConvSet) provide(cs *callState, f *structField) (bool, error) {
	log := cs.Logger.With("field", f)
	log.Trace("looking for converter")

	var merr error
	for _, c := range s {
		score := c.provides(f)
		log.Trace("converter provides score", "conv", c, "score", score)

		// If this converter can't provide the value we're looking for
		// then skip it.
		if score < 0 {
			continue
		}

		// It can provide it! Try to populate the result.
		result := c.call(cs)
		if err := result.Err(); err != nil {
			merr = multierror.Append(merr, err)
			continue
		}

		// Success. Set the output and we're good to go.
		c.outputState(result, cs)
		return true, nil
	}

	return false, merr
}
