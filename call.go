package argmapper

import (
	"fmt"
	"reflect"

	"github.com/hashicorp/go-multierror"
	"github.com/mitchellh/go-argmapper/internal/graph"
)

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

	// Next, we add the values that we know about. These may overlap with
	// inputs we require but the graph ensures that the same vertices are
	// added only once.
	inputs := map[interface{}]struct{}{}
	inputVertex := g.Add(inputVertex{})
	for k, v := range builder.named {
		input := g.Add(valueVertex{
			Name: k,
			Type: v.Type(),
		})

		g.AddEdge(input, inputVertex)

		inputs[graph.VertexID(input)] = struct{}{}
		vertexValues[input] = v
	}

	// If we have converters, add those
	ConvSet(builder.convs).graph(&g)

	// Next we need to connect our dynamic edges if we have any
	for _, raw := range g.Vertices() {
		v, ok := raw.(valueVertex)
		if !ok {
			continue
		}

		g.AddEdgeWeighted(v, g.Add(templateResultVertex{
			Type: v.Type,
		}), 50)
		g.AddEdgeWeighted(g.Add(inheritNameVertex{
			Type: v.Type,
		}), v, 50)
	}

	// Find all the paths to our function. We initialized visited with
	// our function since the DFS won't visit that.
	visited := map[interface{}]struct{}{graph.VertexID(vertex): struct{}{}}
	g.Reverse().DFS(inputVertex, func(v graph.Vertex, next func() error) error {
		// Mark this as visited
		visited[graph.VertexID(v)] = struct{}{}

		// If we arrived at an input we have, then we don't go deeper.
		if v == vertex {
			return nil
		}

		return next()
	})
	println(g.String())
	println("--")

	// Go through all the graph vertices and remove the verticies
	// that weren't visited. These are not part of any path we could
	// take to our function and therefore we don't want to ever call
	// them.
	for _, v := range g.Vertices() {
		if _, ok := visited[graph.VertexID(v)]; !ok {
			g.Remove(v)
		}
	}
	println(g.String())

	topo := g.Reverse().KahnSort()
	println(fmt.Sprintf("%s", topo))
	distTo, edgeTo := g.TopoShortestPath(topo)
	println("DIST", fmt.Sprintf("%#v", distTo))
	println("TO", fmt.Sprintf("%s", edgeTo[graph.VertexID(vertex)]))
	println("PATH-----------")
	for current := edgeTo[graph.VertexID(vertex)]; current != nil; current = edgeTo[graph.VertexID(current)] {
		println(graph.VertexName(current))
	}
	println("PATH-----------")

	// Setup our call state
	state := &callState{
		Named:   map[string]reflect.Value{},
		Inherit: map[reflect.Type]reflect.Value{},
	}

	// Let's walk the graph and print out our paths
	for _, current := range g.Reverse().KahnSort() {
		// Depending on the type of vertex, we execute
		switch v := current.(type) {
		case valueVertex:
			// We have a value.
			if val, ok := vertexValues[v]; ok {
				state.Named[v.Name] = val
			}

		case convVertex:
			// Call the function. We don't need to specify any converters
			// here, we only specify our state because the graph
			// should guarantee that we have exactly what we need.
			result := v.Conv.call(state)
			if err := result.Err(); err != nil {
				return Result{buildErr: err}
			}

			// Get the result
			v.Conv.outputValues(result, state)

		case funcVertex:
			return v.Func.call(state)

		default:
			panic(fmt.Sprintf("unknown vertex: %v", current))
		}
	}

	panic("Call graph never arrived at function")
}

// call -- the unexported version of Call -- calls the function directly
// with the given named arguments. This skips the whole graph creation
// step by requiring args satisfy all required arguments.
func (f *Func) call(state *callState) Result {
	// Initialize the struct we'll be populating
	var buildErr error
	structVal := f.input.New()
	for k, f := range f.input.fields {
		v, ok := state.Named[k]
		if !ok {
			// No match, look for a dynamic value.
			for inheritType, val := range state.Inherit {
				if inheritType.AssignableTo(f.Type) {
					v = val
					ok = true
					break
				}
			}
		}
		if !ok {
			buildErr = multierror.Append(buildErr, fmt.Errorf(
				"argument cannot be satisfied: %s", k))
			continue
		}

		structVal.FieldNamed(k).Set(v)
	}

	// If there was an error setting up the struct, then report that.
	if buildErr != nil {
		return Result{buildErr: buildErr}
	}

	// Call our function
	out := f.fn.Call([]reflect.Value{structVal.Value()})
	return Result{out: out}
}

type callState struct {
	Named   map[string]reflect.Value
	Inherit map[reflect.Type]reflect.Value
}
