package argmapper

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/mitchellh/go-argmapper/internal/graph"
)

// Redefine returns a new func where the requirements are what is missing to
// satisfy the original function given the arguments here. Therefore, args
// may be incomplete, and this will return a function that only depends
// on the missing arguments.
//
// filter is a filter to define what inputs
func (f *Func) Redefine(opts ...Arg) (*Func, error) {
	builder, err := newArgBuilder(opts...)
	if err != nil {
		return nil, err
	}

	// We have to tell our builder that we're redefining. This changes
	// how the graph is constructed slightly.
	builder.redefining = true

	// Get our log we'll use for logging
	log := builder.logger

	// Get our call graph
	g, _, vertexF, vertexI, err := f.callGraph(builder)
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
		case *valueVertex:
			if !v.Value.IsValid() {
				v.Value = reflect.Zero(v.Type)
			}

		case *typedArgVertex:
			v.Value = reflect.Zero(v.Type)

		case *funcVertex:
			// Copy the func since we're going to modify a field in it.
			fCopy := *v.Func
			v.Func = &fCopy

			// Modify the function to be a zero producing function.
			fCopy.fn = fCopy.zeroFunc()
		}
	}

	// Get the topological sort. We only need this so that we can start
	// calculating shortest path. We'll use shortest path information to
	// determine the ideal path from our inputs to the function.
	topo := g.Reverse().KahnSort()
	log.Trace("topological sort", "sort", topo)

	// Build our call state and attempt to reach our target which is our
	// function. This will recursively reach various conversion targets
	// as necessary.
	state := newCallState()
	if err := f.reachTarget(log, &g, topo, vertexF, state); err != nil {
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
	structType := reflect.StructOf(sf)

	// Build our output type which just matches our function today.
	out := make([]reflect.Type, f.fn.Type().NumOut())
	for i := range out {
		out[i] = f.fn.Type().Out(i)
	}

	// If we don't have an error type, add that
	if len(out) == 0 || out[len(out)-1] != errType {
		out = append(out, errType)
	}

	// Build our function type and implementation.
	fnType := reflect.FuncOf([]reflect.Type{structType}, out, false)
	fn := reflect.MakeFunc(fnType, func(args []reflect.Value) []reflect.Value {
		v := args[0]

		// Get our value set. Our args are guaranteed to be a struct.
		set, err := newValueSetFromStruct(structType)
		if err != nil {
			panic(err)
		}

		// Copy our options
		callArgs := make([]Arg, len(opts))
		copy(callArgs, opts)

		// Setup our values
		for name, f := range set.namedFields {
			callArgs = append(callArgs, Named(name, v.Field(f.Index).Interface()))
		}
		for _, f := range set.typedFields {
			callArgs = append(callArgs, Typed(v.Field(f.Index).Interface()))
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

		// Otherwise, return our values! We have to append the zero error
		// value because our defined function always returns an error value.
		return append(result.out, reflect.Zero(errType))
	})

	return NewFunc(fn.Interface())
}

// zeroFunc returns a function implementation that outputs the zero
// value for all of its known outputs. This is used in the redefine graph
// execution so we can determine what inputs are required to reach an output.
func (f *Func) zeroFunc() reflect.Value {
	t := f.output
	fn := f.fn.Type()
	return reflect.MakeFunc(fn, func(args []reflect.Value) []reflect.Value {
		// Create our struct type and set all the fields to zero
		v := t.New()
		for _, f := range t.namedFields {
			v.Field(f.Index).Set(reflect.Zero(f.Type))
		}
		for _, f := range t.typedFields {
			v.Field(f.Index).Set(reflect.Zero(f.Type))
		}

		// Get our result. If we're expecting an error value, return nil for that.
		result := v.CallIn()
		if len(result) < fn.NumOut() {
			result = append(result, reflect.Zero(errType))
		}

		return result
	})
}
