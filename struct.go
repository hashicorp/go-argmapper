package argmapper

import (
	"fmt"
	"reflect"
	"strings"
)

type structType struct {
	typ    reflect.Type
	fields map[string]*structField
}

type structField struct {
	Index int
	Type  reflect.Type
}

func newStructType(typ reflect.Type) (*structType, error) {
	// Verify our value is a struct
	if typ.Kind() != reflect.Struct {
		return nil, fmt.Errorf("struct expected, got %s", typ.Kind())
	}

	// We will accumulate our results here
	result := &structType{
		typ:    typ,
		fields: make(map[string]*structField),
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
					options[v] = "1"
				} else {
					options[v[:idx]] = v[idx+1:]
				}
			}
		}

		// Record it
		result.fields[name] = &structField{
			Index: i,
			Type:  sf.Type,
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

type structValue struct {
	typ   *structType
	value reflect.Value
}

func (v *structValue) Value() reflect.Value {
	return v.value
}

func (v *structValue) Set(n string, val reflect.Value) {
	v.value.Field(v.typ.fields[n].Index).Set(val)
}
