package argmapper

import (
	"fmt"
	"reflect"
	"strings"
)

//go:generate stringer -type=ValueKind

// ValueSet tracks the values either accepted or returned as part of
// a function or converter.
//
// Internally, argmapper converts all functions to a signature of
// `func(Struct) (Struct, error)`. This lets the internals simplify a lot
// by expecting to only set struct fields. On the edges (when calling functions
// or returning values) we convert to and from the true expected arguments.
type ValueSet struct {
	// structType is a struct that contains all the settable values.
	structType reflect.Type

	// values is the set of values that this ValueSet contains. namedValues,
	// typedValues, etc. are convenience maps for looking up values more
	// easily.
	values      []Value
	namedValues map[string]Value
	typedValues map[reflect.Type]Value

	// isLifted is if this represents a lifted struct. A lifted struct
	// is one where we automatically converted flat argument lists to
	// structs.
	isLifted bool
}

// Value represents an input or output of a Func. In normal operation, you
// do not need to interact with Value objects directly. This structure
// is exposed for users who are trying to introspect on functions or manually
// build functions. This is an advanced operation.
//
// A Value represents multiple types of values depending on what fields are
// set. Please read the documentation carefully and use the exported methods
// to assist with checking value types.
type Value struct {
	// valueInternal is the internal information for the value. This is only
	// set and used by ValueSet.
	valueInternal

	// Name is the name of the value. This may be empty if this is a type-only
	// value. If the name is set, then we will satisfy this input with an arg
	// with this name and type.
	Name string

	// Type is the type of the value. This must be set.
	Type reflect.Type

	// Subtype is a key that specifies a unique "subtype" for the type.
	// This can be used to identify dynamic values such as protobuf Any types
	// where the full type isn't available. This is optional. For full details
	// on subtype matching see the package docs.
	Subtype string
}

// ValueKind is returned by Value.Kind to designate what kind of value this is:
// a value expecting a type and name, a value with just type matching, etc.
type ValueKind uint

const (
	ValueInvalid ValueKind = iota
	ValueNamed
	ValueTyped
)

type valueInternal struct {
	// index is the struct field index for the ValueSet on which to set values.
	index int
}

func newValueSet(count int, get func(int) reflect.Type) (*ValueSet, error) {
	// If there are no arguments, then return an empty value set.
	if count == 0 {
		return &ValueSet{}, nil
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
		t := get(i)
		if isStruct(t) {
			return nil, fmt.Errorf("can't mix argmapper.Struct and non-struct values")
		}

		sf = append(sf, reflect.StructField{
			Name: fmt.Sprintf("V__Type_%d", i),
			Type: t,
			Tag:  reflect.StructTag(`argmapper:",typeOnly"`),
		})
	}

	t, err := newValueSetFromStruct(reflect.StructOf(sf))
	if err != nil {
		return nil, err
	}

	t.isLifted = true
	return t, nil
}

func newValueSetFromStruct(typ reflect.Type) (*ValueSet, error) {
	// Verify our value is a struct
	if typ.Kind() != reflect.Struct {
		return nil, fmt.Errorf("struct expected, got %s", typ.Kind())
	}

	// We will accumulate our results here
	result := &ValueSet{
		structType:  typ,
		values:      []Value{},
		namedValues: map[string]Value{},
		typedValues: map[reflect.Type]Value{},
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
		if _, ok := options["typeOnly"]; ok {
			name = ""
		}

		// Record it
		value := Value{
			Name:    name,
			Type:    sf.Type,
			Subtype: options["subtype"],
			valueInternal: valueInternal{
				index: i,
			},
		}

		result.values = append(result.values, value)
		switch value.Kind() {
		case ValueNamed:
			result.namedValues[value.Name] = value

		case ValueTyped:
			result.typedValues[value.Type] = value
		}
	}

	return result, nil
}

// Values returns the values in this ValueSet. This result is not mutable.
func (t *ValueSet) Values() []Value {
	return t.values
}

// Func returns a new Func that calls back into f with the values it receives
// when it is called.
func (t *ValueSet) Func(f func(vs map[Value]interface{}) error) (*Func, error) {
	return nil, nil
}

// New returns a new structValue that can be used for value population.
func (t *ValueSet) New() *structValue {
	result := &structValue{typ: t}
	if t.structType != nil {
		result.value = reflect.New(t.structType).Elem()
	}

	return result
}

// lifted returns true if this field is lifted.
func (t *ValueSet) lifted() bool {
	return t.isLifted
}

func (t *ValueSet) empty() bool {
	return t.structType == nil
}

// result takes the result that matches this struct type and adapts it
// if necessary (if the struct type is lifted or so on).
func (t *ValueSet) result(r Result) Result {
	// If we aren't lifted, we return the result as-is.
	if !t.lifted() {
		return r
	}

	// If we are lifted, then we need to translate the output arguments
	// to their proper types in a struct.
	structOut := reflect.New(t.structType).Elem()
	for _, f := range t.typedValues {
		structOut.Field(f.index).Set(r.out[f.index])
	}

	r.out = []reflect.Value{structOut}
	return r
}

func (v *Value) Kind() ValueKind {
	if v.Name != "" {
		return ValueNamed
	}

	return ValueTyped
}

type structValue struct {
	typ   *ValueSet
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
	result := make([]reflect.Value, len(v.typ.typedValues))
	for _, f := range v.typ.typedValues {
		result[f.index] = v.value.Field(f.index)
	}

	return result
}
