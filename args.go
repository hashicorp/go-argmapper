package argmapper

import (
	"reflect"
	"strings"

	"github.com/hashicorp/go-hclog"
	"github.com/mitchellh/go-argmapper/internal/graph"
)

// Arg is an option to Func.Call that sets the state for the function call.
// This can be a direct named arg or a converter that could be used if
// necessary to reach the target.
type Arg func(*argBuilder) error

type argBuilder struct {
	logger hclog.Logger
	named  map[string]reflect.Value
	typed  map[reflect.Type]reflect.Value
	convs  []*Func
}

// Named specifies a named argument with the given value. This will satisfy
// any requirement where the name matches AND the value is assignable to
// the struct.
func Named(n string, v interface{}) Arg {
	return func(a *argBuilder) error {
		a.named[strings.ToLower(n)] = reflect.ValueOf(v)
		return nil
	}
}

// Typed specifies a typed argument with the given value. This will satisfy
// any requirement where the type is assignable to a required value. The name
// can be anything of the required value.
func Typed(v interface{}) Arg {
	return func(a *argBuilder) error {
		rv := reflect.ValueOf(v)
		a.typed[rv.Type()] = rv
		return nil
	}
}

// WithConvFunc specifies one or more converters to use if necessary.
// A converter will be used if an argument type doesn't match exactly.
func WithConvFunc(fs ...interface{}) Arg {
	return func(a *argBuilder) error {
		for _, f := range fs {
			conv, err := NewFunc(f)
			if err != nil {
				return err
			}

			a.convs = append(a.convs, conv)
		}

		return nil
	}
}

func (b *argBuilder) graph(g *graph.Graph, root graph.Vertex) []graph.Vertex {
	var result []graph.Vertex

	// Add our named inputs
	for k, v := range b.named {
		// Add the input
		input := g.AddOverwrite(&valueVertex{
			Name:  k,
			Type:  v.Type(),
			Value: v,
		})

		// Input depends on the input root
		g.AddEdge(input, root)

		// Track
		result = append(result, input)
	}

	// Add our typed inputs
	for t, v := range b.typed {
		// Add the input
		input := g.AddOverwrite(&typedOutputVertex{
			Type:  t,
			Value: v,
		})

		// Input depends on the input root
		g.AddEdge(input, root)

		// Track
		result = append(result, input)
	}

	return result
}
