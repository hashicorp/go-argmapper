package argmapper

import (
	"fmt"
	"reflect"
	"strings"
)

type structType struct {
	typ    reflect.Type
	fields map[string]*structField

	// inheritName is the list of fields with inheritName set. These
	// are not listed in "fields" since they are nameless and instead
	// just match whatever name of a matching type.
	inheritName map[string]*structField
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
		typ:         typ,
		fields:      make(map[string]*structField),
		inheritName: make(map[string]*structField),
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

		if v, ok := options["inheritName"]; ok {
			result.inheritName[v] = field
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

func (t *structType) copy() *structType {
	fields := map[string]*structField{}
	for k, v := range t.fields {
		fields[k] = v
	}

	inheritName := map[string]*structField{}
	for k, v := range t.inheritName {
		inheritName[k] = v
	}

	return &structType{
		typ:         t.typ,
		fields:      fields,
		inheritName: inheritName,
	}
}

// inherit populates the inherited fields. mapping is a map where the key is
// the inherited group key and the value is the name of the input to
// look for.
func (t *structType) inherit(mapping map[string]string) *structType {
	result := t.copy()
	for k, v := range mapping {
		if f, ok := result.inheritName[k]; ok {
			delete(result.inheritName, k)
			result.fields[v] = f
		}
	}

	return result
}

type structValue struct {
	typ   *structType
	value reflect.Value
}

func (v *structValue) Value() reflect.Value {
	return v.value
}

func (v *structValue) FieldNamed(n string) reflect.Value {
	return v.Field(v.typ.fields[n].Index)
}

func (v *structValue) Field(idx int) reflect.Value {
	return v.value.Field(idx)
}
