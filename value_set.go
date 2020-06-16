package argmapper

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/hashicorp/go-argmapper/internal/graph"
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
	values      []*Value
	namedValues map[string]*Value
	typedValues map[reflect.Type]*Value

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

	// Value is the known value. This is only ever set if using Func.Redefine
	// with an input that was given. Otherwise, this value is invalid.
	Value reflect.Value
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

// NewValueSet creates a ValueSet from a list of expected values.
//
// This is primarily used alongside BuildFunc to dynamically build a Func.
func NewValueSet(vs []Value) (*ValueSet, error) {
	// Build a dynamic struct based on the value list. The struct is
	// only used for input/output mapping.
	var sf []reflect.StructField
	sf = append(sf, reflect.StructField{
		Name:      "Struct",
		Type:      structMarkerType,
		Anonymous: true,
	})
	for i, v := range vs {
		if isStruct(v.Type) {
			return nil, fmt.Errorf("can't have argmapper.Struct values with custom ValueSet building")
		}

		// TODO(mitchellh): error on duplicate names, types

		// Build our tag.
		tags := []string{""}
		if v.Name == "" {
			tags = append(tags, "typeOnly")
		}
		if v.Subtype != "" {
			tags = append(tags, fmt.Sprintf("subtype=%s", v.Subtype))
		}
		tag := reflect.StructTag(fmt.Sprintf(`argmapper:"%s"`, strings.Join(tags, ",")))

		switch v.Kind() {
		case ValueNamed:
			sf = append(sf, reflect.StructField{
				Name: strings.ToUpper(v.Name),
				Type: v.Type,
				Tag:  tag,
			})

		case ValueTyped:
			sf = append(sf, reflect.StructField{
				Name: fmt.Sprintf("V__Type_%d", i),
				Type: v.Type,
				Tag:  tag,
			})

		default:
			panic("unknown kind")
		}
	}

	return newValueSetFromStruct(reflect.StructOf(sf))
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
		values:      []*Value{},
		namedValues: map[string]*Value{},
		typedValues: map[reflect.Type]*Value{},
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

		result.values = append(result.values, &value)
		switch value.Kind() {
		case ValueNamed:
			result.namedValues[value.Name] = &value

		case ValueTyped:
			result.typedValues[value.Type] = &value
		}
	}

	return result, nil
}

// Values returns the values in this ValueSet. This does not return
// pointers so any modifications to the values will not impact any values
// in this set. Please call Named, Typed, etc. directly to make modifications.
func (vs *ValueSet) Values() []Value {
	result := make([]Value, len(vs.values))
	for i, v := range vs.values {
		result[i] = *v
	}
	return result
}

// Named returns a pointer to the value with the given name, or nil if
// it doesn't exist.
func (vs *ValueSet) Named(n string) *Value {
	return vs.namedValues[n]
}

// Typed returns a pointer to the value with the given type, or nil
// if it doesn't exist. If there is no typed value directly, a random
// type with the matching subtype will be chosen. If you want an exact
// match with no subtype, use TypedSubtype.
func (vs *ValueSet) Typed(t reflect.Type) *Value {
	// TODO: subtype
	return vs.typedValues[t]
}

// TypedSubtype returns a pointer to the value that matches the type
// and subtype exactly.
func (vs *ValueSet) TypedSubtype(t reflect.Type, st string) *Value {
	for _, v := range vs.values {
		if v.Type == t && v.Subtype == st {
			return v
		}
	}

	return nil
}

// Signature returns the type signature that this ValueSet will map to/from.
// This is used for making dynamic types with reflect.FuncOf to take or return
// this valueset.
func (vs *ValueSet) Signature() []reflect.Type {
	if !vs.lifted() {
		// This happens if the value has no values at all. In this case,
		// our signature is also empty.
		if vs.structType == nil {
			return nil
		}

		return []reflect.Type{vs.structType}
	}

	result := make([]reflect.Type, len(vs.typedValues))
	for _, v := range vs.typedValues {
		result[v.index] = v.Type
	}

	return result
}

// SignatureValues returns the values that match the Signature type list,
// based on the values set in this set. If a value isn't set, the zero
// value is used.
func (vs *ValueSet) SignatureValues() []reflect.Value {
	// If typ is nil then there is no values
	if vs.structType == nil {
		return nil
	}

	// If we're lifted, we just return directly based on values
	if vs.lifted() {
		result := make([]reflect.Value, len(vs.typedValues))
		for _, v := range vs.typedValues {
			result[v.index] = v.valueOrZero()
		}

		return result
	}

	// Not lifted, meaning we return a struct
	structOut := reflect.New(vs.structType).Elem()
	for _, f := range vs.values {
		structOut.Field(f.index).Set(f.valueOrZero())
	}

	return []reflect.Value{structOut}
}

// FromSignature sets the values in this ValueSet based on the values list.
// The values list must match the type signature returned from vs.Signature.
// This usually comes from calling a function directly.
func (vs *ValueSet) FromSignature(values []reflect.Value) error {
	// If we're lifted, then first set the values onto the struct.
	if vs.lifted() {
		// If we are lifted, then we need to translate the output arguments
		// to their proper types in a struct.
		structOut := reflect.New(vs.structType).Elem()
		for _, f := range vs.typedValues {
			structOut.Field(f.index).Set(values[f.index])
		}

		values = []reflect.Value{structOut}
	}

	// Get our first result which should be our struct
	structVal := values[0]
	for i, v := range vs.values {
		vs.values[i].Value = structVal.Field(v.index)
	}

	return nil
}

// FromResult sets the values in this set based on a Result. This will
// return an error if the result represents an error.
func (vs *ValueSet) FromResult(r Result) error {
	if err := r.Err(); err != nil {
		return err
	}

	return vs.FromSignature(r.out)
}

// New returns a new structValue that can be used for value population.
func (t *ValueSet) newStructValue() *structValue {
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

func newValueFromVertex(v graph.Vertex) *Value {
	switch v := v.(type) {
	case *valueVertex:
		return &Value{
			Name:    v.Name,
			Type:    v.Type,
			Subtype: v.Subtype,
			Value:   v.Value,
		}

	case *typedOutputVertex:
		return &Value{
			Type:    v.Type,
			Subtype: v.Subtype,
			Value:   v.Value,
		}
	}

	return nil
}

// Arg returns an Arg that can be used with Func.Call to send this value.
// This only works if the Value's Value field is set.
func (v *Value) Arg() Arg {
	switch v.Kind() {
	case ValueNamed:
		return NamedSubtype(v.Name, v.Value.Interface(), v.Subtype)

	case ValueTyped:
		return TypedSubtype(v.Value.Interface(), v.Subtype)

	default:
		panic("unknown kind: " + v.Kind().String())
	}
}

// Kind returns the ValueKind that this Value represents.
func (v *Value) Kind() ValueKind {
	if v.Name != "" {
		return ValueNamed
	}

	return ValueTyped
}

func (v *Value) String() string {
	switch v.Kind() {
	case ValueNamed:
		return fmt.Sprintf("%q (type: %s)", v.Name, v.Type)

	case ValueTyped:
		return fmt.Sprintf("type %s", v.Type)

	default:
		return fmt.Sprintf("%#v", v)
	}
}

func (v *Value) valueOrZero() reflect.Value {
	if !v.Value.IsValid() {
		return reflect.Zero(v.Type)
	}

	return v.Value
}

func (v *Value) vertex() graph.Vertex {
	switch v.Kind() {
	case ValueNamed:
		return &valueVertex{
			Name:    v.Name,
			Type:    v.Type,
			Subtype: v.Subtype,
		}

	case ValueTyped:
		return &typedArgVertex{
			Type:    v.Type,
			Subtype: v.Subtype,
		}

	default:
		panic(fmt.Sprintf("unknown value kind: %s", v.Kind()))
	}
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
