package argmapper

import (
	"fmt"
	"reflect"

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

func (c *Conv) inherit(mapping map[string]string) {
}

// outputValues extracts the output from the given Result. The Result must
// be a result of calling Call on this exact Conv. Specifying any other
// Result is undefined and will likely result in panics.
func (c *Conv) outputValues(r Result, state *callState) {
	// Get our struct
	v := r.out[0]

	// Go through the fields we know about
	for name, f := range c.output.fields {
		state.Named[name] = v.Field(f.Index)
	}

	// Set our inherited name values
	for _, f := range c.output.inheritName {
		state.Inherit[f.Type] = v.Field(f.Index)
	}
}

// ConvSet is a set of converters.
type ConvSet []*Conv

func (cs ConvSet) graph(g *graph.Graph) {
	// Go through all our convs and create the vertices for our inputs and outputs
	for _, conv := range cs {
		vertex := g.Add(convVertex{
			Conv: conv,
		})

		// Add all our inputs and add an edge from the func to the input
		for k, f := range conv.input.fields {
			g.AddEdge(vertex, g.Add(valueVertex{
				Name: k,
				Type: f.Type,
			}))
		}
		for _, f := range conv.input.inheritName {
			g.AddEdgeWeighted(vertex, g.Add(inheritNameVertex{
				Type: f.Type,
			}), 50)
		}

		// Add all our outputs
		for k, f := range conv.output.fields {
			g.AddEdge(g.Add(valueVertex{
				Name: k,
				Type: f.Type,
			}), vertex)
		}
		for _, f := range conv.output.inheritName {
			g.AddEdgeWeighted(g.Add(templateResultVertex{
				Type: f.Type,
			}), vertex, 50)
		}
	}
}

type inheritNameVertex struct {
	Type reflect.Type
}

func (v *inheritNameVertex) Hashcode() interface{} {
	return fmt.Sprintf("-> */%s", v.Type.String())
}

type templateResultVertex struct {
	Type reflect.Type
}

func (v *templateResultVertex) Hashcode() interface{} {
	return fmt.Sprintf("<- */%s", v.Type.String())
}

type valueVertex struct {
	Name string
	Type reflect.Type
}

func (v *valueVertex) Hashcode() interface{} {
	return fmt.Sprintf("%s/%s", v.Name, v.Type.String())
}

type convVertex struct {
	Conv *Conv
}

func (v *convVertex) Hashcode() interface{} { return v.Conv }
func (v convVertex) String() string         { return "conv: " + v.Conv.fn.String() }

type funcVertex struct {
	Func *Func
}

func (v *funcVertex) Hashcode() interface{} { return v.Func }
func (v funcVertex) String() string         { return "func: " + v.Func.fn.String() }

type inputVertex struct{}

func (v inputVertex) String() string { return "input root" }

var (
	_ graph.VertexHashable = (*convVertex)(nil)
	_ graph.VertexHashable = (*funcVertex)(nil)
	_ graph.VertexHashable = (*valueVertex)(nil)
)
