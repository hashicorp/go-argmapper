package argmapper

import (
	"fmt"
	"reflect"

	"github.com/hashicorp/go-argmapper/internal/graph"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-multierror"
)

// Call calls the function. Use the various Arg functions to set the state
// for the function call. More details on how Call works are on the Func
// struct documentation directly.
func (f *Func) Call(opts ...Arg) Result {
	// Build up our args
	builder, buildErr := f.argBuilder(opts...)
	if buildErr != nil {
		return resultError(buildErr)
	}
	log := builder.logger
	log.Trace("call")

	// Build our call graph
	g, vertexRoot, vertexF, _, err := f.callGraph(builder)
	if err != nil {
		return resultError(err)
	}

	// Reach our target function to get our arguments, performing any
	// conversions necessary.
	argMap, err := f.reachTarget(log, &g, vertexRoot, vertexF, newCallState(), false)
	if err != nil {
		return resultError(err)
	}

	return f.callDirect(log, argMap)
}

// callGraph builds the common graph used by Call, Redefine, etc.
func (f *Func) callGraph(args *argBuilder) (
	g graph.Graph,
	vertexRoot graph.Vertex,
	vertexF graph.Vertex,
	vertexI []graph.Vertex,
	err error,
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
	vertexFreq := g.OutEdges(vertexF)

	// Next, we add "inputs", which are the given named values that
	// we already know about. These are tracked as "vertexI".
	vertexI = args.graph(log, &g, vertexRoot)

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
		g.AddEdgeWeighted(v, g.Add(&typedOutputVertex{
			Type: v.Type,
		}), weightTyped)

		// We always add an edge from the arg to the value, whether it
		// has one or not. In the next step, we'll prune any typed arguments
		// that already have a satisfied value.
		g.AddEdgeWeighted(g.Add(&typedArgVertex{
			Type: v.Type,
		}), v, weightTyped)

		// If this value has a subtype, we add an edge for the subtype
		if v.Subtype != "" {
			g.AddEdgeWeighted(g.Add(&typedArgVertex{
				Type:    v.Type,
				Subtype: v.Subtype,
			}), v, weightTyped)
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
			Type:    v.Type,
			Subtype: v.Subtype,
		}), weightTyped)
	}

	// Typed output vertices that are interfaces can be satisfied by
	// interface implementations. i.e. `out: error` -> `out: *fmt.Error`.
	for _, raw := range g.Vertices() {
		v, ok := raw.(*typedOutputVertex)
		if !ok || v.Type.Kind() != reflect.Interface {
			continue
		}

		for _, raw2 := range g.Vertices() {
			if raw == raw2 {
				continue
			}

			v2, ok := raw2.(*typedOutputVertex)
			if !ok || !v2.Type.Implements(v.Type) {
				continue
			}

			g.AddEdgeWeighted(v, v2, weightTyped)
		}
	}

	// All named values that have no subtype can take a value from
	// any other named value that has a subtype.
	for _, raw := range g.Vertices() {
		v, ok := raw.(*valueVertex)
		if !ok || v.Subtype != "" || v.Value.IsValid() {
			continue
		}

		for _, raw := range g.Vertices() {
			v2, ok := raw.(*valueVertex)
			if !ok || v2.Type != v.Type || v2.Subtype == "" {
				continue
			}

			g.AddEdgeWeighted(v, v2, weightTyped)
		}
	}

	// All typed values that have no subtype can take a value from
	// any output with a subtype.
	for _, raw := range g.Vertices() {
		v, ok := raw.(*typedArgVertex)
		if !ok || v.Subtype != "" {
			continue
		}

		for _, raw := range g.Vertices() {
			v2, ok := raw.(*typedOutputVertex)
			if !ok || v2.Type != v.Type || v2.Subtype == "" {
				continue
			}

			g.AddEdgeWeighted(v, v2, weightTypedOtherSubtype)
		}
	}

	// All typed values that have no subtype can take a value from
	// any output with a subtype.
	for _, raw := range g.Vertices() {
		v, ok := raw.(*typedArgVertex)
		if !ok || v.Subtype == "" {
			continue
		}

		for _, raw := range g.Vertices() {
			v2, ok := raw.(*typedOutputVertex)
			if !ok || v2.Type != v.Type || v2.Subtype != "" {
				continue
			}

			g.AddEdgeWeighted(v, v2, weightTypedOtherSubtype)
		}
	}

	// If we're redefining based on inputs, then we also want to
	// go through and set a path from our input root to all the values
	// in the graph. This lets us pick the shortest path through based on
	// any valid input.
	if args.redefining {
		for _, raw := range g.Vertices() {
			var value Value

			// We are looking for either a value or a typed arg. Both
			// of these represent "inputs" to a function.
			v, ok := raw.(*valueVertex)
			if ok {
				value = Value{
					Name:    v.Name,
					Type:    v.Type,
					Subtype: v.Subtype,
					Value:   v.Value,
				}
			}
			if !ok {
				v, ok := raw.(*typedArgVertex)
				if !ok {
					continue
				}

				value = Value{
					Type:    v.Type,
					Subtype: v.Subtype,
					Value:   v.Value,
				}
			}

			// For redefining, the caller can setup filters to determine
			// what inputs they're capable of providing. If any filter
			// says it is possible, then we take the value.
			include := true
			if args.filterInput != nil && !args.filterInput(value) {
				log.Trace("excluding input due to failed filter", "value", value)
				include = false
				continue
			}

			if include {
				// Connect this to the root, since it is a potential input to
				// satisfy a function that gets us to redefine.
				g.AddEdge(raw, vertexRoot)
			}
		}
	}

	log.Trace("full graph (may have cycles)", "graph", g.String())

	// Next we do a DFS from each input A in I to the function F.
	// This gives us the full set of reachable nodes from our inputs
	// and at most to F. Using this information, we can prune any nodes
	// that are guaranteed to be unused.
	//
	// DFS from the input root and record what we see. We have to reverse the
	// graph here because we typically have out edges pointing to
	// requirements, but we're going from requirements (inputs) to
	// the function.
	visited := map[interface{}]struct{}{
		// We must keep the root. Since we're starting from the root we don't
		// "visit" it. But we must keep it for shortest path calculations. If
		// we don't keep it, our shortest path calculations are from some
		// other zero index topo sort value.
		graph.VertexID(vertexRoot): struct{}{},
	}
	g.Reverse().DFS(vertexRoot, func(v graph.Vertex, next func() error) error {
		visited[graph.VertexID(v)] = struct{}{}

		if v == vertexF {
			return nil
		}
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

	// Go through all our inputs. If any aren't in the graph any longer
	// it means there is no possible path to that input so it cannot be
	// satisfied.
	err = nil
	for _, req := range vertexFreq {
		if g.Vertex(graph.VertexID(req)) == nil {
			name := graph.VertexName(req)
			switch v := req.(type) {
			case *valueVertex:
				name = fmt.Sprintf("%q of type %s", v.Name, v.Type.String())
				if v.Subtype != "" {
					name += fmt.Sprintf(" (subtype: %q)", v.Subtype)
				}

			case *typedArgVertex:
				name = fmt.Sprintf("type %s", v.Type.String())
				if v.Subtype != "" {
					name += fmt.Sprintf(" (subtype: %q)", v.Subtype)

				}
			}

			err = multierror.Append(err, fmt.Errorf(
				"argument cannot be satisfied: %s", name))
		}
	}

	return
}

// reachTarget executes the the given funcVertex by ensuring we satisfy
// all the inbound arguments first and then calling it.
func (f *Func) reachTarget(
	log hclog.Logger,
	g *graph.Graph,
	root graph.Vertex,
	target graph.Vertex,
	state *callState,
	redefine bool,
) (map[interface{}]reflect.Value, error) {
	log.Trace("reachTarget", "target", target)

	// argMap will store all the values that this target depends on.
	argMap := map[interface{}]reflect.Value{}

	// Look at the out edges, since these are the requirements for the conv
	// and determine which inputs we need values for. If we have a value
	// already then we skip the target because we assume it is already in
	// the state.
	var vertexT []graph.Vertex
	for _, out := range g.OutEdges(target) {
		skip := false
		switch v := out.(type) {
		case *rootVertex:
			// If we see a root vertex, then that means that this target
			// has no dependencies.
			skip = true

		case *typedArgVertex:
			if v.Value.IsValid() {
				skip = true
				argMap[graph.VertexID(out)] = v.Value
			}
		}

		// If we're skipping because we have this value already, then
		// note that we're using this input in the input set.
		if skip {
			state.InputSet[graph.VertexID(out)] = out
			continue
		}

		log.Trace("conv is missing an input", "input", out)
		vertexT = append(vertexT, out)
	}

	if len(vertexT) == 0 {
		log.Trace("conv satisfied")
		return argMap, nil
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

		// Recalculate the shortest path information since we may changed
		// the graph above.
		_, edgeTo := currentG.Reverse().Dijkstra(root)

		// With the latest shortest paths, let's add the path for this target.
		paths[i] = currentG.EdgeToPath(current, edgeTo)
		log.Trace("path for target", "target", current, "path", paths[i])

		// Get the input
		input := paths[i][0]
		if _, ok := input.(*rootVertex); ok && len(paths[i]) > 1 {
			input = paths[i][1]
		}

		// Store our input used
		state.InputSet[graph.VertexID(input)] = input

		// When we're redefining, we always set the initial input to
		// the zero value because we assume we'll have access to it. We
		// can assume this because that is the whole definition of redefining.
		if redefine {
			switch v := input.(type) {
			case *valueVertex:
				if !v.Value.IsValid() {
					v.Value = reflect.Zero(v.Type)
				}

			case *typedArgVertex:
				v.Value = reflect.Zero(v.Type)
			}
		}
	}

	// Go through each path
	for _, path := range paths {
		// finalValue will be set to our final value that we see when walking.
		// This will be set as the value for this required input.
		var finalValue reflect.Value

		for pathIdx, vertex := range path {
			log.Trace("executing node", "current", vertex)
			switch v := vertex.(type) {
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

					finalValue = v.Value
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

				finalValue = v.Value

			case *typedOutputVertex:
				// If our last node was another typed output, then we take
				// that value.
				if pathIdx > 0 {
					prev := path[pathIdx-1]
					if r, ok := prev.(*typedOutputVertex); ok {
						log.Trace("setting node value", "value", r.Value)
						v.Value = r.Value
					}
				}

				// Last value
				state.Value = v.Value

				// Set the typed value we can read from.
				state.TypedValue[v.Type] = v.Value

			case *funcVertex:
				// Reach our arguments if they aren't already.
				funcArgMap, err := f.reachTarget(
					log, //log.Named(graph.VertexName(v)),
					g,
					root,
					v,
					state,
					redefine,
				)
				if err != nil {
					return nil, err
				}

				// Call our function.
				result := v.Func.callDirect(log, funcArgMap)
				if err := result.Err(); err != nil {
					return nil, err
				}

				// Update our graph nodes and continue
				v.Func.outputValues(result, g.InEdges(v), state)

			default:
				panic(fmt.Sprintf("unknown vertex: %v", v))
			}
		}

		// We should always have a final value, because our execution to
		// this point only leads up to this value.
		if !finalValue.IsValid() {
			panic(fmt.Sprintf("didn't reach a final value for path: %#v", path))
		}

		// We store the final value in the input map.
		log.Trace("final value", "vertex", path[len(path)-1], "value", finalValue.Interface())
		argMap[graph.VertexID(path[len(path)-1])] = finalValue
	}

	// Reached our goal
	return argMap, nil
}

// call -- the unexported version of Call -- calls the function directly
// with the given named arguments. This skips the whole graph creation
// step by requiring args satisfy all required arguments.
func (f *Func) callDirect(log hclog.Logger, argMap map[interface{}]reflect.Value) Result {
	// Initialize the struct we'll be populating
	var buildErr error
	structVal := f.input.newStructValue()
	for _, val := range f.input.values {
		arg, ok := argMap[graph.VertexID(val.vertex())]
		if !ok {
			buildErr = multierror.Append(buildErr, fmt.Errorf(
				"argument cannot be satisfied: %s", val.String()))
			continue
		}

		structVal.Field(val.index).Set(arg)
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
