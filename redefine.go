package argmapper

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/hashicorp/go-argmapper/internal/graph"
	"github.com/hashicorp/go-multierror"
)

// Redefine returns a new func where the requirements are what is missing to
// satisfy the original function given the arguments here. Therefore, args
// may be incomplete, and this will return a function that only depends
// on the missing arguments.
//
// Redefine also allows the usage of FilterInput and FilterOutput Arg
// values. These can be used to further restrict what values can be provided
// as an input or returned as an output, respectively. This can be used
// for example to try to redefine a function to only take Go primitives.
// In the case where Filter is used, converters must be specified that
// enable going to and from filtered values.
//
// Currently, FilterOutput will just return an error if the functions
// outputs don't match what is expected. In the future, we plan on enabling
// FilterOutput to also map through converters to return the desired matches.
//
// If it is impossible to redefine the function according to the given
// constraints, an error will be returned.
func (f *Func) Redefine(opts ...Arg) (*Func, error) {
	// First we check the outputs since we currently only error if the outputs
	// do not match the filter. In the future, we'll do conversions here too.
	if err := f.redefineOutputs(opts...); err != nil {
		return nil, err
	}

	// Redefine our inputs to get the struct type that we'll take in the new function.
	inputStruct, err := f.redefineInputs(opts...)
	if err != nil {
		return nil, err
	}

	// Build our output type which just matches our function today.
	out := make([]reflect.Type, f.fn.Type().NumOut())
	for i := range out {
		out[i] = f.fn.Type().Out(i)
	}

	// hasErr tells us whether out originally had an error output. We need
	// this to construct the proper return value in the dynamic func below.
	hasErr := true

	// If we don't have an error type, add that
	if len(out) == 0 || out[len(out)-1] != errType {
		out = append(out, errType)
		hasErr = false
	}

	// Build our function type and implementation.
	fnType := reflect.FuncOf([]reflect.Type{inputStruct}, out, false)
	fn := reflect.MakeFunc(fnType, func(args []reflect.Value) []reflect.Value {
		v := args[0]

		// Get our value set. Our args are guaranteed to be a struct.
		set, err := newValueSetFromStruct(inputStruct)
		if err != nil {
			panic(err)
		}

		// Copy our options
		callArgs := make([]Arg, len(opts))
		copy(callArgs, opts)

		// Setup our values
		for name, f := range set.namedValues {
			callArgs = append(callArgs, Named(name, v.Field(f.index).Interface()))
		}
		for _, f := range set.typedValues {
			callArgs = append(callArgs, Typed(v.Field(f.index).Interface()))
		}

		// Call
		result := f.Call(callArgs...)

		// If we had an error, then we return the error. We always define
		// our new functions to return a final error type so set that and
		// return.
		if err := result.Err(); err != nil {
			retval := make([]reflect.Value, len(out))
			for i, t := range out {
				retval[i] = reflect.Zero(t)
			}

			retval[len(retval)-1] = reflect.ValueOf(err)
			return retval
		}

		out := result.out
		if !hasErr {
			// If we didn't originally have an error value, then we
			// append the zero value since we always return an error
			// as the final result from this dynamic func.
			out = append(result.out, reflect.Zero(errType))
		}

		return out
	})

	return NewFunc(fn.Interface())
}

// redefineInputs is called by Redefine to determine the input struct type
// expected based on the Redefine arguments. This returns a struct type
// (as reflect.Type) that represents the new input structure for the
// redefined function.
func (f *Func) redefineInputs(opts ...Arg) (reflect.Type, error) {
	builder, err := f.argBuilder(opts...)
	if err != nil {
		return nil, err
	}

	// We have to tell our builder that we're redefining. This changes
	// how the graph is constructed slightly.
	builder.redefining = true

	// Get our log we'll use for logging
	log := builder.logger

	// Get our call graph
	g, vertexRoot, vertexF, vertexI, err := f.callGraph(builder)
	if err != nil {
		return nil, err
	}

	// We go through the call graph and modify the functions to be no-ops
	// that just set the output values to zero values. This will let our
	// redefine process "call" each of our converters as if they work
	// perfectly and then we can determine what inputs are required by
	// just calling our target function and checking state.InputSet.
	for _, v := range g.Vertices() {
		switch v := v.(type) {
		case *funcVertex:
			// Copy the func since we're going to modify a field in it.
			fCopy := *v.Func
			v.Func = &fCopy

			// Modify the function to be a zero producing function.
			fCopy.fn = fCopy.zeroFunc()
		}
	}

	// Build our call state and attempt to reach our target which is our
	// function. This will recursively reach various conversion targets
	// as necessary.
	state := newCallState()
	if _, err := f.reachTarget(log, &g, vertexRoot, vertexF, state, true); err != nil {
		return nil, err
	}

	// Determine our map of inputs
	inputsProvided := map[interface{}]struct{}{}
	for _, v := range vertexI {
		inputsProvided[graph.VertexID(v)] = struct{}{}
	}

	// Build our required value
	var sf []reflect.StructField
	sf = append(sf, reflect.StructField{
		Name:      "Struct",
		Type:      structMarkerType,
		Anonymous: true,
	})
	for k, v := range state.InputSet {
		log.Trace("input", "value", v)
		if _, ok := inputsProvided[k]; ok {
			continue
		}

		switch v := v.(type) {
		case *valueVertex:
			sf = append(sf, reflect.StructField{
				Name: strings.ToUpper(v.Name),
				Type: v.Type,
			})

		case *typedArgVertex:
			sf = append(sf, reflect.StructField{
				Name: fmt.Sprintf("V__Type_%d", len(sf)),
				Type: v.Type,
				Tag:  reflect.StructTag(`argmapper:",typeOnly"`),
			})
		}
	}

	return reflect.StructOf(sf), nil
}

// redefineOutputs redefines the outputs of the function in accordance
// with FilterOutput.
//
// NOTE(mitchellh): today, we just validate the outputs. In the future,
// we'll chain converters to reach a desired output.
func (f *Func) redefineOutputs(opts ...Arg) error {
	builder, err := newArgBuilder(opts...)
	if err != nil {
		return err
	}

	if builder.filterOutput == nil {
		return nil
	}

	err = nil
	for _, v := range f.Output().Values() {
		if !builder.filterOutput(v) {
			err = multierror.Append(err, fmt.Errorf(
				"output %s does not satisfy output filter", v.String()))
		}
	}

	return err
}

// zeroFunc returns a function implementation that outputs the zero
// value for all of its known outputs. This is used in the redefine graph
// execution so we can determine what inputs are required to reach an output.
func (f *Func) zeroFunc() reflect.Value {
	t := f.output
	fn := f.fn.Type()
	return reflect.MakeFunc(fn, func(args []reflect.Value) []reflect.Value {
		// Create our struct type and set all the fields to zero
		v := t.newStructValue()
		for _, f := range t.namedValues {
			v.Field(f.index).Set(reflect.Zero(f.Type))
		}
		for _, f := range t.typedValues {
			v.Field(f.index).Set(reflect.Zero(f.Type))
		}

		// Get our result. If we're expecting an error value, return nil for that.
		result := v.CallIn()
		if len(result) < fn.NumOut() {
			result = append(result, reflect.Zero(errType))
		}

		return result
	})
}
