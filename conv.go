package argmapper

import (
	"fmt"

	"github.com/mitchellh/go-argmapper/internal/graph"
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

	output, err := newStructType(ft.NumOut(), ft.Out)
	if err != nil {
		return nil, err
	}

	return &Conv{
		Func:   fn,
		output: output,
	}, nil
}

// outputValues extracts the output from the given Result. The Result must
// be a result of calling Call on this exact Conv. Specifying any other
// Result is undefined and will likely result in panics.
func (c *Conv) outputValues(r Result, vs []graph.Vertex, state *callState) {
	// Get our struct
	structVal := c.output.result(r).out[0]
	println("OUTPUT", fmt.Sprintf("%#v", structVal.Interface()))

	// TODO: we need to deconstruct LIFTED VALUES

	// Go through our out edges to find all our results so we can update
	// the graph nodes with our values. Along the way, we also update our
	// total call state.
	for _, v := range vs {
		switch v := v.(type) {
		case *valueVertex:
			// Set the value on the vertex. During the graph walk, we'll
			// set the Named value.
			v.Value = structVal.Field(c.output.fields[v.Name].Index)

		case *typedOutputVertex:
			// Get our field with the same name
			field := c.output.typedFields[v.Name]

			// Determine our target name
			target := state.Mapping[v.Name]

			v.ValueName = target.Name
			v.Value = structVal.Field(field.Index)
		}
	}
}

// ConvSet is a set of converters.
type ConvSet []*Conv

func (cs ConvSet) graph(g *graph.Graph) {
	// Go through all our convs and create the vertices for our inputs and outputs
	for _, conv := range cs {
		vertex := g.Add(&convVertex{
			Conv: conv,
		})

		// Add all our inputs and add an edge from the func to the input
		for k, f := range conv.input.fields {
			g.AddEdge(vertex, g.Add(&valueVertex{
				Name: k,
				Type: f.Type,
			}))
		}
		for _, f := range conv.input.typedFields {
			g.AddEdgeWeighted(vertex, g.Add(&typedArgVertex{
				Type: f.Type,
			}), 50)
		}

		// Add all our outputs
		for k, f := range conv.output.fields {
			g.AddEdge(g.Add(&valueVertex{
				Name: k,
				Type: f.Type,
			}), vertex)
		}
		for _, f := range conv.output.typedFields {
			g.AddEdgeWeighted(g.Add(&typedOutputVertex{
				Type: f.Type,
			}), vertex, 50)
		}
	}
}
