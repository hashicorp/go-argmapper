package argmapper

import (
	"fmt"
	"reflect"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-multierror"
	"github.com/mitchellh/go-argmapper/internal/graph"
)

func (f *Func) callGraph(args *argBuilder) (
	g graph.Graph,
	vertexRoot graph.Vertex,
	vertexF graph.Vertex,
	vertexI []graph.Vertex,
) {
	log := args.logger

	// Create a shared root. Anything reachable from the root is not pruned.
	// This is primarily inputs but may also contain parameterless converters
	// (providers).
	vertexRoot = g.Add(&rootVertex{})

	// Build the graph. The first step is to add our function and all the
	// requirements of the function. We keep track of this in vertexF and
	// vertexT, respectively, because we'll need these later.
	vertexF = f.graph(&g, vertexRoot, false)

	// Next, we add "inputs", which are the given named values that
	// we already know about. These are tracked as "vertexI".
	vertexI = args.graph(&g, vertexRoot)

	// Next, for all values we may have or produce, we need to create
	// the vertices for the type-only value. This lets us say, for example,
	// that an input "A string" satisfies anything that requires only "string".
	for _, raw := range g.Vertices() {
		v, ok := raw.(*valueVertex)
		if !ok {
			continue
		}

		// We only add an edge from the output if we require a value.
		// If we already have a value then we don't need to request one.
		if !v.Value.IsValid() {
			g.AddEdgeWeighted(v, g.Add(&typedOutputVertex{
				Type: v.Type,
			}), weightTyped)
		}

		// We always add an edge from the arg to the value, whether it
		// has one or not. In the next step, we'll prune any typed arguments
		// that already have a satisfied value.
		g.AddEdgeWeighted(g.Add(&typedArgVertex{
			Type: v.Type,
		}), v, weightTyped)
	}

	// If we're redefining based on inputs, then we also want to
	// go through and set a path from our input root to all the values
	// in the graph. This lets us pick the shortest path through based on
	// any valid input.
	if args.redefining {
		for _, raw := range g.Vertices() {
			var typ reflect.Type

			// We are looking for either a value or a typed arg. Both
			// of these represent "inputs" to a function.
			v, ok := raw.(*valueVertex)
			if ok {
				typ = v.Type
			}
			if !ok {
				v, ok := raw.(*typedArgVertex)
				if !ok {
					continue
				}

				typ = v.Type
			}

			// For redefining, the caller can setup filters to determine
			// what inputs they're capable of providing. If any filter
			// says it is possible, then we take the value.
			include := true
			for _, f := range args.filters {
				if !f(typ) {
					include = false
					break
				}
			}

			if include {
				// Connect this to the root, since it is a potential input to
				// satisfy a function that gets us to redefine.
				g.AddEdge(raw, vertexRoot)
			}
		}
	}

	// We need to allow any typed argument to depend on a typed output.
	// This lets two converters chain together.
	for _, raw := range g.Vertices() {
		v, ok := raw.(*typedArgVertex)
		if !ok {
			continue
		}

		g.AddEdgeWeighted(v, g.Add(&typedOutputVertex{
			Type: v.Type,
		}), weightTyped)
	}

	log.Trace("full graph (may have cycles)", "graph", g.String())

	// TODO: explain why
	for _, raw := range g.Vertices() {
		v, ok := raw.(*typedArgVertex)
		if !ok {
			continue
		}

		keep := map[interface{}]struct{}{}
		for _, out := range g.OutEdges(v) {
			if v, ok := out.(*valueVertex); ok && v.Value.IsValid() {
				keep[graph.VertexID(out)] = struct{}{}
				break
			}
		}

		if len(keep) > 0 {
			for _, v := range vertexI {
				keep[graph.VertexID(v)] = struct{}{}
			}

			for _, out := range g.OutEdges(v) {
				if _, ok := keep[graph.VertexID(out)]; !ok {
					g.RemoveEdge(v, out)
				}
			}
		}
	}

	// Next we do a DFS from each input A in I to the function F.
	// This gives us the full set of reachable nodes from our inputs
	// and at most to F. Using this information, we can prune any nodes
	// that are guaranteed to be unused.
	//
	// DFS from the input root and record what we see. We have to reverse the
	// graph here because we typically have out edges pointing to
	// requirements, but we're going from requirements (inputs) to
	// the function.
	visited := map[interface{}]struct{}{graph.VertexID(vertexF): struct{}{}}
	g.Reverse().DFS(vertexRoot, func(v graph.Vertex, next func() error) error {
		if v == vertexF {
			return nil
		}

		visited[graph.VertexID(v)] = struct{}{}
		return next()
	})

	// Remove all the non-visited vertices. After this, what we'll have
	// is a graph that has many paths getting us from inputs to function,
	// but we will have no spurious vertices that are unreachable from our
	// inputs.
	for _, v := range g.Vertices() {
		if _, ok := visited[graph.VertexID(v)]; !ok {
			g.Remove(v)
		}
	}
	log.Trace("graph after input DFS", "graph", g.String())

	return
}

// Call calls the function. Use the various Arg functions to set the state
// for the function call.
func (f *Func) Call(opts ...Arg) Result {
	// Build up our args
	builder, buildErr := newArgBuilder(opts...)
	if buildErr != nil {
		return resultError(buildErr)
	}

	var g graph.Graph
	log := builder.logger

	// Create a shared root. Anything reachable from the root is not pruned.
	// This is primarily inputs but may also contain parameterless converters
	// (providers).
	vertexRoot := g.Add(&rootVertex{})

	// Build the graph. The first step is to add our function and all the
	// requirements of the function. We keep track of this in vertexF and
	// vertexT, respectively, because we'll need these later.
	vertexF := f.graph(&g, vertexRoot, false)

	// If we have converters, add those. See ConvSet.graph for more details.
	for _, f := range builder.convs {
		f.graph(&g, vertexRoot, true)
	}

	// Next, we add "inputs", which are the given named values that
	// we already know about. These are tracked as "vertexI".
	vertexI := builder.graph(&g, vertexRoot)

	// Next, for all values we may have or produce, we need to create
	// the vertices for the type-only value. This lets us say, for example,
	// that an input "A string" satisfies anything that requires only "string".
	for _, raw := range g.Vertices() {
		v, ok := raw.(*valueVertex)
		if !ok {
			continue
		}

		// We only add an edge from the output if we require a value.
		// If we already have a value then we don't need to request one.
		if !v.Value.IsValid() {
			g.AddEdgeWeighted(v, g.Add(&typedOutputVertex{
				Type: v.Type,
			}), weightTyped)
		}

		// We always add an edge from the arg to the value, whether it
		// has one or not. In the next step, we'll prune any typed arguments
		// that already have a satisfied value.
		g.AddEdgeWeighted(g.Add(&typedArgVertex{
			Type: v.Type,
		}), v, weightTyped)
	}

	// We need to allow any typed argument to depend on a typed output.
	// This lets two converters chain together.
	for _, raw := range g.Vertices() {
		v, ok := raw.(*typedArgVertex)
		if !ok {
			continue
		}

		g.AddEdgeWeighted(v, g.Add(&typedOutputVertex{
			Type: v.Type,
		}), weightTyped)
	}

	log.Trace("full graph (may have cycles)", "graph", g.String())

	// TODO: explain why
	for _, raw := range g.Vertices() {
		v, ok := raw.(*typedArgVertex)
		if !ok {
			continue
		}

		keep := map[interface{}]struct{}{}
		for _, out := range g.OutEdges(v) {
			if v, ok := out.(*valueVertex); ok && v.Value.IsValid() {
				keep[graph.VertexID(out)] = struct{}{}
				break
			}
		}

		if len(keep) > 0 {
			for _, v := range vertexI {
				keep[graph.VertexID(v)] = struct{}{}
			}

			for _, out := range g.OutEdges(v) {
				if _, ok := keep[graph.VertexID(out)]; !ok {
					g.RemoveEdge(v, out)
				}
			}
		}
	}

	// Next we do a DFS from each input A in I to the function F.
	// This gives us the full set of reachable nodes from our inputs
	// and at most to F. Using this information, we can prune any nodes
	// that are guaranteed to be unused.
	//
	// DFS from the input root and record what we see. We have to reverse the
	// graph here because we typically have out edges pointing to
	// requirements, but we're going from requirements (inputs) to
	// the function.
	visited := map[interface{}]struct{}{graph.VertexID(vertexF): struct{}{}}
	g.Reverse().DFS(vertexRoot, func(v graph.Vertex, next func() error) error {
		if v == vertexF {
			return nil
		}

		visited[graph.VertexID(v)] = struct{}{}
		return next()
	})

	// Remove all the non-visited vertices. After this, what we'll have
	// is a graph that has many paths getting us from inputs to function,
	// but we will have no spurious vertices that are unreachable from our
	// inputs.
	for _, v := range g.Vertices() {
		if _, ok := visited[graph.VertexID(v)]; !ok {
			g.Remove(v)
		}
	}
	log.Trace("graph after input DFS", "graph", g.String())

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
		return resultError(err)
	}

	return f.callDirect(log, state)
}

// reachTarget executes the the given funcVertex by ensuring we satisfy
// all the inbound arguments first and then calling it.
func (f *Func) reachTarget(
	log hclog.Logger,
	g *graph.Graph,
	topo graph.TopoOrder,
	target graph.Vertex,
	state *callState,
) error {
	// Look at the out edges, since these are the requirements for the conv
	// and determine which inputs we need values for. If we have a value
	// already then we skip the target because we assume it is already in
	// the state.
	var vertexT []graph.Vertex
	for _, out := range g.OutEdges(target) {
		skip := false
		switch v := out.(type) {
		case *typedArgVertex:
			skip = v.Value.IsValid()
		}

		if !skip {
			log.Trace("conv is missing an input", "input", out)
			vertexT = append(vertexT, out)
		}
	}

	if len(vertexT) == 0 {
		log.Trace("conv satisfied")
		return nil
	}

	paths := make([][]graph.Vertex, len(vertexT))
	for i, current := range vertexT {
		currentG := g

		// For value vertices, we discount any other values that share the
		// same name. This lets our shortest paths prefer matching through
		// same-named arguments.
		if currentValue, ok := current.(*valueVertex); ok {
			currentG = currentG.Copy()
			for _, raw := range currentG.Vertices() {
				if v, ok := raw.(*valueVertex); ok && v.Name == currentValue.Name {
					for _, src := range currentG.InEdges(raw) {
						currentG.AddEdgeWeighted(src, raw, weightMatchingName)
					}
				}
			}
		}

		// Get the shortest path data. We need to reverse the graph here since
		// the topo sort is from the reversal as well. We have to calculate
		// the shortest path for each vertexT value because we may change
		// edge weights above. We can reuse the topo value because the shape
		// of the graph is not changing.
		_, edgeTo := currentG.Reverse().TopoShortestPath(topo.Until(current))

		// With the latest shortest paths, let's add the path for this target.
		paths[i] = currentG.EdgeToPath(current, edgeTo)
		log.Trace("path for target", "target", current, "path", paths[i])

		// Store our input used
		state.InputSet[graph.VertexID(paths[i][0])] = paths[i][0]
	}

	// Go through each path
	remaining := len(paths)
	idx := 0
	for remaining > 0 {
		path := paths[idx]
		if len(path) == 0 {
			idx++
			continue
		}

		pathIdx := 0
		for pathIdx = 0; pathIdx < len(path); pathIdx++ {
			log := log.With("current", path[pathIdx])
			log.Trace("executing node")

			switch v := path[pathIdx].(type) {
			case *rootVertex:
				// Do nothing

			case *valueVertex:
				// Store the last viewed vertex in our path state
				state.Value = v.Value

				if pathIdx > 0 {
					prev := path[pathIdx-1]
					if r, ok := prev.(*typedOutputVertex); ok {
						log.Trace("setting node value", "value", r.Value)
						v.Value = r.Value
					}
				}

				// If we have a valid value set, then put it on our named list.
				if v.Value.IsValid() {
					state.NamedValue[v.Name] = v.Value
				}

			case *typedArgVertex:
				// If we have a value set on the state then we set that to this
				// value. This is true in every Call case but is always false
				// for Redefine.
				if state.Value.IsValid() && state.Value.Type().AssignableTo(v.Type) {
					// The value of this is the last value vertex we saw. The graph
					// walk should ensure this is the correct type.
					v.Value = state.Value
				}

				// Setup our mapping so that we know that this wildcard
				// maps to this name.
				state.TypedValue[v.Type] = v.Value

			case *typedOutputVertex:
				// Last value
				state.Value = v.Value

				// Set the typed value we can read from.
				state.TypedValue[v.Type] = v.Value

			case *funcVertex:
				// Reach our arguments if they aren't already.
				if err := f.reachTarget(
					log.Named(graph.VertexName(v)),
					g,
					topo,
					v,
					state,
				); err != nil {
					return err
				}

				// Call our function.
				result := v.Func.callDirect(log, state)
				if err := result.Err(); err != nil {
					return err
				}

				// Update our graph nodes and continue
				v.Func.outputValues(result, g.InEdges(v), state)

			default:
				panic(fmt.Sprintf("unknown vertex: %v", v))
			}
		}

		paths[idx] = path[pathIdx:]
		if len(paths[idx]) == 0 {
			remaining--
		}
		idx++
	}

	// Reached our goal
	return nil
}

// call -- the unexported version of Call -- calls the function directly
// with the given named arguments. This skips the whole graph creation
// step by requiring args satisfy all required arguments.
func (f *Func) callDirect(log hclog.Logger, state *callState) Result {
	// Initialize the struct we'll be populating
	var buildErr error
	structVal := f.input.New()
	for k, f := range f.input.namedFields {
		v, ok := state.NamedValue[k]
		if !ok {
			buildErr = multierror.Append(buildErr, fmt.Errorf(
				"argument cannot be satisfied: %s", k))
			continue
		}

		structVal.Field(f.Index).Set(v)
	}

	for _, f := range f.input.typedFields {
		v, ok := state.TypedValue[f.Type]
		if !ok {
			buildErr = multierror.Append(buildErr, fmt.Errorf(
				"typed argument cannot be satisfied at index %d (type %s)",
				f.Index, f.Type.String()))
			continue
		}

		structVal.Field(f.Index).Set(v)
	}

	// If there was an error setting up the struct, then report that.
	if buildErr != nil {
		return Result{buildErr: buildErr}
	}

	// Call our function
	in := structVal.CallIn()
	for i, arg := range in {
		log.Trace("argument", "idx", i, "value", arg.Interface())
	}

	out := f.fn.Call(in)
	return Result{out: out}
}

// callState is the shared state for the execution of a single call.
type callState struct {
	// NamedValue holds the current table of known named values.
	NamedValue map[string]reflect.Value

	// TypedValue holds the current table of assigned typed values.
	TypedValue map[reflect.Type]reflect.Value

	// Value is the last seen value vertex. This state is preserved so
	// we can set the typedVertex values properly.
	Value reflect.Value

	// TODO
	InputSet map[interface{}]graph.Vertex
}

func newCallState() *callState {
	return &callState{
		NamedValue: map[string]reflect.Value{},
		TypedValue: map[reflect.Type]reflect.Value{},
		InputSet:   map[interface{}]graph.Vertex{},
	}
}
