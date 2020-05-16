package graph

// Vertex can be anything.
type Vertex interface{}

// VertexHashable is an optional interface that can be implemented to specify
// an alternate hash code for a Vertex. If this isnt implemented, Go interface
// equality is used.
type VertexHashable interface {
	Hashcode() interface{}
}

// hashcode returns the hashcode for a Vertex.
func hashcode(v interface{}) interface{} {
	if h, ok := v.(VertexHashable); ok {
		return h.Hashcode()
	}

	return v
}
