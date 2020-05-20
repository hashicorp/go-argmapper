package argmapper

import (
	"fmt"
	"reflect"
	"strings"
)

// valueSet tracks the values either accepted or returned as part of
// a function or converter.
//
// Internally, argmapper converts all functions to a signature of
// `func(Struct) (Struct, error)`. This lets the internals simplify a lot
// by expecting to only set struct fields. On the edges (when calling functions
// or returning values) we convert to and from the true expected arguments.
type valueSet struct {
	// structType is a struct that contains all the settable values.
	structType reflect.Type

	// namedFields are fields that have a specifc name. The key is the name.
	namedFields map[string]*structField

	// typedFields are the fields that are only type-matching and can accept
	// values of any name as long as the type matches.
	typedFields map[reflect.Type]*structField

	// isLifted is if this represents a lifted struct. A lifted struct
	// is one where we automatically converted flat argument lists to
	// structs.
	isLifted bool
}

type structField struct {
	// Index is the index using reflect.Value.Field that can be used to
	// set this field on the structType. This makes it easy to quickly set
	// any value.
	Index int

	// Type is the type of this field.
	Type reflect.Type
}

func newValueSet(count int, get func(int) reflect.Type) (*valueSet, error) {
	// If there are no arguments, then return an empty value set.
	if count == 0 {
		return &valueSet{}, nil
	}

	// If we have exactly one argument, let's check if its a struct. If
	// it is then we treat it as the full value.
	if count == 1 {
		if t := get(0); isStruct(t) {
			return newValueSetFromStruct(t)
		}
	}

	// We need to lift the arguments into a "struct".
	var sf []reflect.StructField
	for i := 0; i < count; i++ {
		sf = append(sf, reflect.StructField{
			Name: fmt.Sprintf("A__Type_%d", i),
			Type: get(i),

			// TODO: won't work with multiple
			Tag: reflect.StructTag(`argmapper:",typeOnly"`),
		})
	}

	t, err := newValueSetFromStruct(reflect.StructOf(sf))
	if err != nil {
		return nil, err
	}

	t.isLifted = true
	return t, nil
}

func newValueSetFromStruct(typ reflect.Type) (*valueSet, error) {
	// Verify our value is a struct
	if typ.Kind() != reflect.Struct {
		return nil, fmt.Errorf("struct expected, got %s", typ.Kind())
	}

	// We will accumulate our results here
	result := &valueSet{
		structType:  typ,
		namedFields: make(map[string]*structField),
		typedFields: make(map[reflect.Type]*structField),
	}

	// Go through the fields and record them all
	for i := 0; i < typ.NumField(); i++ {
		sf := typ.Field(i)

		// Ignore unexported fields and our struct marker
		if sf.PkgPath != "" || isStructField(sf) {
			continue
		}

		// name is the name of the value to inject.
		name := sf.Name

		// Parse out the tag if there is one
		var options map[string]string
		if tag := sf.Tag.Get("argmapper"); tag != "" {
			parts := strings.Split(tag, ",")

			// If we have a name set, then override the name
			if parts[0] != "" {
				name = parts[0]
			}

			// If we have fields set after the comma, then we want to
			// parse those as values.
			options = make(map[string]string)
			for _, v := range parts[1:] {
				idx := strings.Index(v, "=")
				if idx == -1 {
					options[v] = ""
				} else {
					options[v[:idx]] = v[idx+1:]
				}
			}
		}

		// Name is always lowercase
		name = strings.ToLower(name)

		// Record it
		field := &structField{
			Index: i,
			Type:  sf.Type,
		}

		if _, ok := options["typeOnly"]; ok {
			result.typedFields[field.Type] = field
		} else {
			result.namedFields[name] = field
		}
	}

	return result, nil
}

// New returns a new structValue that can be used for value population.
func (t *valueSet) New() *structValue {
	result := &structValue{typ: t}
	if t.structType != nil {
		result.value = reflect.New(t.structType).Elem()
	}

	return result
}

// lifted returns true if this field is lifted.
func (t *valueSet) lifted() bool {
	return t.isLifted
}

func (t *valueSet) empty() bool {
	return t.structType == nil
}

// result takes the result that matches this struct type and adapts it
// if necessary (if the struct type is lifted or so on).
func (t *valueSet) result(r Result) Result {
	// If we aren't lifted, we return the result as-is.
	if !t.lifted() {
		return r
	}

	// If we are lifted, then we need to translate the output arguments
	// to their proper types in a struct.
	structOut := reflect.New(t.structType).Elem()
	for _, f := range t.typedFields {
		structOut.Field(f.Index).Set(r.out[f.Index])
	}

	r.out = []reflect.Value{structOut}
	return r
}

type structValue struct {
	typ   *valueSet
	value reflect.Value
}

func (v *structValue) Field(idx int) reflect.Value {
	return v.value.Field(idx)
}

func (v *structValue) CallIn() []reflect.Value {
	// If typ is nil then there is no inputs
	if v.typ.structType == nil {
		return nil
	}

	// If this is not lifted, return it as-is.
	if !v.typ.lifted() {
		return []reflect.Value{v.value}
	}

	// This is lifted, so we need to unpack them in order.
	result := make([]reflect.Value, len(v.typ.typedFields))
	for _, f := range v.typ.typedFields {
		result[f.Index] = v.value.Field(f.Index)
	}

	return result
}
