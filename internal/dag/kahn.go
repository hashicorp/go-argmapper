package dag

func (g *AcyclicGraph) KahnSort() []Vertex {
	/*
	   L ← Empty list that will contain the sorted elements
	   S ← Set of all nodes with no incoming edge

	   while S is non-empty do
	       remove a node n from S
	       add n to tail of L
	       for each node m with an edge e from n to m do
	           remove edge e from the graph
	           if m has no other incoming edges then
	               insert m into S

	   if graph has edges then
	       return error   (graph has at least one cycle)
	   else
	       return L   (a topologically sorted order)
	*/

	vertices := g.Vertices()

	// L ← Empty list that will contain the sorted elements
	L := make([]Vertex, 0, len(vertices))

	// S ← Set of all nodes with no incoming edge
	S := []Vertex{}
	for _, v := range vertices {
		if g.UpEdges(v).Len() == 0 {
			S = append(S, v)
		}
	}

	// while S is non-empty do
	for len(S) > 0 {
		// remove a node n from S
		n := S[len(S)-1]
		S = S[:len(S)-1]

		// add n to tail of L
		L = append(L, n)

		// for each node m with an edge e from n to m do
		for _, raw := range g.DownEdges(n).List() {
			m := raw.(Vertex)

			// remove edge e from the graph
			g.RemoveEdge(BasicEdge(n, m))

			// if m has no other incoming edges then
			if g.UpEdges(m).Len() == 0 {
				// insert m into S
				S = append(S, m)
			}
		}
	}

	// if graph has edges then
	//   return error   (graph has at least one cycle)
	if len(g.Edges()) > 0 {
		panic("graph has cycles")
	}

	return L
}
