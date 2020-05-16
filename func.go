package argmapper

import (
	"fmt"
	"reflect"
	"unicode"
	"unicode/utf8"

	"github.com/hashicorp/go-multierror"
	"github.com/mitchellh/go-argmapper/internal/graph"
)

type Func struct {
	fn    reflect.Value
	input *structType
}

func NewFunc(f interface{}) (*Func, error) {
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

	structTyp, err := newStructType(typ)
	if err != nil {
		return nil, err
	}

	return &Func{
		fn:    fv,
		input: structTyp,
	}, nil
}

func (f *Func) Call(opts ...Arg) Result {
	// Build up our args
	builder := &argBuilder{
		named: make(map[string]reflect.Value),
	}
	for _, opt := range opts {
		opt(builder)
	}

	// Start building our graph. The first step is to add our own vertex.
	// Then we go through all the named inputs we have and add them to the
	// graph, with an edge from our function to the inputs we require.
	var g graph.Graph
	vertex := g.Add(funcVertex{
		Func: f,
	})
	for k, f := range f.input.fields {
		g.AddEdge(vertex, g.Add(valueVertex{
			Name: k,
			Type: f.Type,
		}))
	}

	// Values is the built up list of values we know about
	vertexValues := map[graph.Vertex]reflect.Value{}
	inValues := map[string]reflect.Value{}

	// Next, we add the values that we know about. These may overlap with
	// inputs we require but the graph ensures that the same vertices are
	// added only once.
	inputs := map[interface{}]struct{}{}
	for k, v := range builder.named {
		input := g.Add(valueVertex{
			Name: k,
			Type: v.Type(),
		})

		inputs[graph.VertexID(input)] = struct{}{}
		vertexValues[input] = v
	}

	// If we have converters, add those
	ConvSet(builder.convs).graph(&g)
	println(g.String())

	// Find all the paths to our function
	visited := map[interface{}]struct{}{}
	g.DFS(vertex, func(v graph.Vertex, next func() error) error {
		// Mark this as visited
		visited[graph.VertexID(v)] = struct{}{}

		// If we arrived at an input we have, then we don't go deeper.
		if _, ok := inputs[graph.VertexID(v)]; ok {
			return nil
		}

		return next()
	})

	// Let's walk the graph and print out our paths
	println(fmt.Sprintf("%s", g.Reverse().KahnSort()))
	for _, current := range g.Reverse().KahnSort() {
		// Depending on the type of vertex, we execute
		switch v := current.(type) {
		case valueVertex:
			// We have a value.
			if val, ok := vertexValues[v]; ok {
				inValues[v.Name] = val
			}

		case convVertex:
			// Call the function. We don't need to specify any converters
			// here, we only specify our "inValues" because the graph
			// should guarantee that we have exactly what we need.
			result := v.Conv.call(inValues)
			if err := result.Err(); err != nil {
				return Result{buildErr: err}
			}

			// Get the result
			v.Conv.outputValues(result, inValues)

		case funcVertex:
			return v.Func.call(inValues)

		default:
			panic(fmt.Sprintf("unknown vertex: %v", current))
		}
	}

	panic("Call graph never arrived at function")
}

// call -- the unexported version of Call -- calls the function directly
// with the given named arguments. This skips the whole graph creation
// step by requiring args satisfy all required arguments.
func (f *Func) call(args map[string]reflect.Value) Result {
	// Initialize the struct we'll be populating
	var buildErr error
	structVal := f.input.New()
	for k, _ := range f.input.fields {
		v, ok := args[k]
		if !ok {
			buildErr = multierror.Append(buildErr, fmt.Errorf(
				"argument cannot be satisfied: %s", k))
			continue
		}

		structVal.Field(k).Set(v)
	}

	// If there was an error setting up the struct, then report that.
	if buildErr != nil {
		return Result{buildErr: buildErr}
	}

	// Call our function
	out := f.fn.Call([]reflect.Value{structVal.Value()})
	return Result{out: out}
}

func firstToLower(s string) string {
	if len(s) > 0 {
		r, size := utf8.DecodeRuneInString(s)
		if r != utf8.RuneError || size > 1 {
			lo := unicode.ToLower(r)
			if lo != r {
				s = string(lo) + s[size:]
			}
		}
	}
	return s
}

// errType is used for comparison in Spec
var errType = reflect.TypeOf((*error)(nil)).Elem()
