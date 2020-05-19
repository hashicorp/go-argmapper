package argmapper

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

var liftedNameMap = map[int]string{}

const liftedNameMax = 64

func init() {
	for i := 0; i < liftedNameMax; i++ {
		liftedNameMap[i] = "A__" + strconv.Itoa(i)
	}
}

type structType struct {
	typ    reflect.Type
	fields map[string]*structField

	// typedFields is the list of fields with typeOnly set. These
	// are not listed in "fields" since they are nameless and instead
	// just match whatever name of a matching type.
	typedFields map[string]*structField

	// isLifted is if this represents a lifted struct. A lifted struct
	// is one where we automatically converted flat argument lists to
	// structs.
	isLifted bool
}

type structField struct {
	Index int
	Type  reflect.Type
}

func newStructType(count int, get func(int) reflect.Type) (*structType, error) {
	// We require functions take one argument. In reality we should be able
	// to accept zero argument functions but there are a lot of assumptions
	// we haven't resolved yet. This shouldn't be too difficult to support.
	if count == 0 {
		return nil, fmt.Errorf("function must take at least one argument")
	}

	// If we have exactly one argument, let's check if its a struct. If
	// it is then we treat it as the full value.
	if count == 1 {
		t := get(0)
		if t.Kind() == reflect.Struct {
			return newStructTypeFromStruct(t)
		}
	}

	// We need to lift the arguments into a "struct".
	var sf []reflect.StructField
	for i := 0; i < count; i++ {
		sf = append(sf, reflect.StructField{
			Name: liftedNameMap[i],
			Type: get(i),

			// TODO: won't work with multiple
			Tag: reflect.StructTag(`argmapper:",typeOnly"`),
		})
	}

	t, err := newStructTypeFromStruct(reflect.StructOf(sf))
	if err != nil {
		return nil, err
	}

	t.isLifted = true
	return t, nil
}

func newStructTypeFromStruct(typ reflect.Type) (*structType, error) {
	// Verify our value is a struct
	if typ.Kind() != reflect.Struct {
		return nil, fmt.Errorf("struct expected, got %s", typ.Kind())
	}

	// We will accumulate our results here
	result := &structType{
		typ:         typ,
		fields:      make(map[string]*structField),
		typedFields: make(map[string]*structField),
	}

	// Go through the fields and record them all
	for i := 0; i < typ.NumField(); i++ {
		sf := typ.Field(i)

		// Ignore unexported fields
		if sf.PkgPath != "" {
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

		if v, ok := options["typeOnly"]; ok {
			v = field.Type.String()
			result.typedFields[v] = field
		} else {
			result.fields[name] = field
		}
	}

	return result, nil
}

// New returns a new structValue that can be used for value population.
func (t *structType) New() *structValue {
	return &structValue{
		typ:   t,
		value: reflect.New(t.typ).Elem(),
	}
}

// lifted returns true if this field is lifted.
func (t *structType) lifted() bool {
	return t.isLifted
}

// result takes the result that matches this struct type and adapts it
// if necessary (if the struct type is lifted or so on).
func (t *structType) result(r Result) Result {
	// If we aren't lifted, we return the result as-is.
	if !t.lifted() {
		return r
	}

	// If we are lifted, then we need to translate the output arguments
	// to their proper types in a struct.
	structOut := reflect.New(t.typ).Elem()
	for _, f := range t.typedFields {
		structOut.Field(f.Index).Set(r.out[f.Index])
	}

	r.out = []reflect.Value{structOut}
	return r
}

func (t *structType) copy() *structType {
	fields := map[string]*structField{}
	for k, v := range t.fields {
		fields[k] = v
	}

	typedFields := map[string]*structField{}
	for k, v := range t.typedFields {
		typedFields[k] = v
	}

	return &structType{
		typ:         t.typ,
		fields:      fields,
		typedFields: typedFields,
	}
}

type structValue struct {
	typ   *structType
	value reflect.Value
}

func (v *structValue) FieldNamed(n string) reflect.Value {
	return v.Field(v.typ.fields[n].Index)
}

func (v *structValue) Field(idx int) reflect.Value {
	return v.value.Field(idx)
}

func (v *structValue) CallIn() []reflect.Value {
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
