package argmapper

type structInterface interface {
	argmapperStruct()
}

// Struct should be embedded into any struct where the parameters are
// populated. This lets argmapper differentiate between arguments
// where you want the full struct provided or fields within the struct.
//
// Example:
//
//   type MyParams {
//     argmapper.Struct
//
//     // A and B will be populated through injection..
//     A, B int
//   }
//
// If the embedded Struct was left out, argmapper would look for
// a full MyParams type to inject.
type Struct struct {
	structInterface
}
