package argmapper

import (
	"fmt"
	"reflect"

	"github.com/mitchellh/go-argmapper/internal/dag"
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
	input, output *structType
}

// NewConv constructs a new converter. See the docs on Conv for more info.
func NewConv(f interface{}) (*Conv, error) {
	fv := reflect.ValueOf(f)
	ft := fv.Type()
	if k := ft.Kind(); k != reflect.Func {
		return nil, fmt.Errorf("fn should be a function, got %s", k)
	}

	// We only accept zero or 1 arguments right now. In the future we
	// could potentially expand this to support multiple args that are
	// all structs we populate but for now lets just simplify this.
	if ft.NumIn() > 1 {
		return nil, fmt.Errorf("function must take one struct arg")
	}

	// Our argument must be a struct
	typ := ft.In(0)
	if typ.Kind() != reflect.Struct {
		return nil, fmt.Errorf("function must take one struct arg")
	}

	// Validate output types.
	if ft.NumOut() != 1 {
		return nil, fmt.Errorf("function must return one or two results")
	}

	out := ft.Out(0)
	if out.Kind() != reflect.Struct {
		return nil, fmt.Errorf("first return value must be a struct or *struct")
	}

	input, err := newStructType(typ)
	if err != nil {
		return nil, err
	}

	output, err := newStructType(out)
	if err != nil {
		return nil, err
	}

	return &Conv{
		input:  input,
		output: output,
	}, nil
}

// ConvSet is a set of converters.
type ConvSet []*Conv

func (cs ConvSet) graph(g *dag.Graph) {
	// Go through all our convs and create the vertices for our inputs and outputs
	for _, conv := range cs {
		vertex := g.Add(convVertex{
			Conv: conv,
		})

		// Add all our inputs and add an edge from the func to the input
		for k, f := range conv.input.fields {
			g.Connect(dag.BasicEdge(vertex, g.Add(valueVertex{
				Name: k,
				Type: f.Type,
			})))
		}

		// Add all our outputs
		for k, f := range conv.input.fields {
			g.Connect(dag.BasicEdge(g.Add(valueVertex{
				Name: k,
				Type: f.Type,
			}), vertex))
		}
	}
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

type funcVertex struct {
	Func *Func
}

func (v *funcVertex) Hashcode() interface{} { return v.Func }
func (v funcVertex) String() string         { return v.Func.fn.String() }

var (
	_ dag.Hashable = (*convVertex)(nil)
	_ dag.Hashable = (*funcVertex)(nil)
	_ dag.Hashable = (*valueVertex)(nil)
)
