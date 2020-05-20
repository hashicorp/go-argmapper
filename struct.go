package argmapper

import "reflect"

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

// structInterface so that users can't just embed any struct{} type.
type structInterface interface {
	argmapperStruct()
}

// isStruct returns true if the given type is a struct that embeds our
// struct marker.
func isStruct(t reflect.Type) bool {
	if t.Kind() != reflect.Struct {
		return false
	}

	for i := 0; i < t.NumField(); i++ {
		if isStructField(t.Field(i)) {
			return true
		}
	}

	return false
}

// isStructField returns true if the given struct field is our struct marker.
func isStructField(f reflect.StructField) bool {
	return f.Anonymous && f.Type == structMarkerType
}

var structMarkerType = reflect.TypeOf((*Struct)(nil)).Elem()
