package argmapper

import (
	"fmt"
	"reflect"

	"github.com/mitchellh/go-argmapper/internal/graph"
)

// Func represents both a target function you want to execute as well as
// a function that can be used to provide values, convert types, etc. for
// calling another Func.
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
type Func struct {
	fn       reflect.Value
	input    *ValueSet
	output   *ValueSet
	callOpts []Arg
}

// NewFunc creates a new Func from the given input function f.
//
// For more details on the format of the function f, see the package docs.
//
// Additional opts can be provided. These will always be set when calling
// Call. Any conflicting arguments given on Call will override these args.
// This can be used to provide some initial values, converters, etc.
func NewFunc(f interface{}, opts ...Arg) (*Func, error) {
	fv := reflect.ValueOf(f)
	ft := fv.Type()
	if k := ft.Kind(); k != reflect.Func {
		return nil, fmt.Errorf("fn should be a function, got %s", k)
	}

	inTyp, err := newValueSet(ft.NumIn(), ft.In)
	if err != nil {
		return nil, err
	}

	// Get our output parameters. If the last parameter is an error type
	// then we don't parse that as the struct information.
	numOut := ft.NumOut()
	if numOut > 1 && ft.Out(numOut-1) == errType {
		numOut -= 1
	}

	outTyp, err := newValueSet(numOut, ft.Out)
	if err != nil {
		return nil, err
	}

	return &Func{
		fn:       fv,
		input:    inTyp,
		output:   outTyp,
		callOpts: opts,
	}, nil
}

// BuildFunc builds a function based on the specified input and output
// value sets. When called, this will call the cb with a valueset matching
// input and output with the argument values set. The cb should return
// a populated ValueSet.
func BuildFunc(input, output *ValueSet, cb func(in, out *ValueSet) error, opts ...Arg) (*Func, error) {
	// Make our function type.
	funcType := reflect.FuncOf(
		input.Signature(),
		append(output.Signature(), errType), // append error so we can return errors
		false,
	)

	// Build our function
	return NewFunc(reflect.MakeFunc(funcType, func(vs []reflect.Value) []reflect.Value {
		// Set our input
		if err := input.FromSignature(vs); err != nil {
			panic(err)
		}

		// Call
		if err := cb(input, output); err != nil {
			panic(err)
		}

		return append(output.SignatureValues(), reflect.Zero(errType))
	}).Interface(), opts...)
}

// Input returns the input ValueSet for this function, representing the values
// that this function requires as input.
func (f *Func) Input() *ValueSet { return f.input }

// Output returns the output ValueSet for this function, representing the values
// that this function produces as an output.
func (f *Func) Output() *ValueSet { return f.output }

// Func returns the function pointer that this Func is built around.
func (f *Func) Func() interface{} {
	return f.fn.Interface()
}

// argBuilder returns the instantiated argBuilder based on the opts provided
// as well as the default opts attached to the func.
func (f *Func) argBuilder(opts ...Arg) (*argBuilder, error) {
	if len(f.callOpts) > 0 {
		optsCopy := make([]Arg, len(opts)+len(f.callOpts))
		copy(optsCopy, f.callOpts)
		copy(optsCopy[len(f.callOpts):], opts)
		opts = optsCopy
	}

	return newArgBuilder(opts...)
}

// graph adds this function to the graph. The given root should be a single
// shared root to the graph, typically a rootVertex. This returns the
// funcVertex created.
//
// includeOutput controls whether to include the output values in the graph.
// This should be true for all intermediary functions but false for the
// target function.
func (f *Func) graph(g *graph.Graph, root graph.Vertex, includeOutput bool) graph.Vertex {
	vertex := g.Add(&funcVertex{
		Func: f,
	})

	// If we take no arguments, we add this function to the root
	// so that it isn't pruned.
	if f.input.empty() {
		g.AddEdge(vertex, root)
	}

	// Add all our inputs and add an edge from the func to the input
	for _, val := range f.input.values {
		switch val.Kind() {
		case ValueNamed:
			g.AddEdge(vertex, g.Add(&valueVertex{
				Name:    val.Name,
				Type:    val.Type,
				Subtype: val.Subtype,
			}))

		case ValueTyped:
			g.AddEdgeWeighted(vertex, g.Add(&typedArgVertex{
				Type:    val.Type,
				Subtype: val.Subtype,
			}), weightTyped)

		default:
			panic(fmt.Sprintf("unknown value kind: %s", val.Kind()))
		}
	}

	if includeOutput {
		// Add all our outputs
		for k, f := range f.output.namedValues {
			g.AddEdge(g.Add(&valueVertex{
				Name:    k,
				Type:    f.Type,
				Subtype: f.Subtype,
			}), vertex)
		}
		for _, f := range f.output.typedValues {
			g.AddEdgeWeighted(g.Add(&typedOutputVertex{
				Type:    f.Type,
				Subtype: f.Subtype,
			}), vertex, weightTyped)
		}
	}

	return vertex
}

// outputValues extracts the output from the given Result. The Result must
// be a result of calling Call on this exact Func. Specifying any other
// Result is undefined and will likely result in panics.
func (f *Func) outputValues(r Result, vs []graph.Vertex, state *callState) {
	// Get our struct
	structVal := f.output.result(r).out[0]

	// Go through our out edges to find all our results so we can update
	// the graph nodes with our values. Along the way, we also update our
	// total call state.
	for _, v := range vs {
		switch v := v.(type) {
		case *valueVertex:
			// Set the value on the vertex. During the graph walk, we'll
			// set the Named value.
			v.Value = structVal.Field(f.output.namedValues[v.Name].index)

		case *typedOutputVertex:
			// Get our field with the same name
			field := f.output.typedValues[v.Type]
			v.Value = structVal.Field(field.index)
		}
	}
}

// errType is used for comparison in Spec
var errType = reflect.TypeOf((*error)(nil)).Elem()
