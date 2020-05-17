package argmapper

import (
	"fmt"
	"reflect"
	"strings"
)

type structType struct {
	typ    reflect.Type
	fields map[string]*structField

	// wildcard is the list of fields with wildcard set. These
	// are not listed in "fields" since they are nameless and instead
	// just match whatever name of a matching type.
	wildcard map[string]*structField
}

type structField struct {
	Index int
	Name  string
	Type  reflect.Type
}

func newStructType(typ reflect.Type) (*structType, error) {
	// Verify our value is a struct
	if typ.Kind() != reflect.Struct {
		return nil, fmt.Errorf("struct expected, got %s", typ.Kind())
	}

	// We will accumulate our results here
	result := &structType{
		typ:      typ,
		fields:   make(map[string]*structField),
		wildcard: make(map[string]*structField),
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

		// Name is always lowercase
		name = strings.ToLower(name)

		// Record it
		field := &structField{
			Index: i,
			Name:  name,
			Type:  sf.Type,
		}

		if v, ok := options["wildcard"]; ok {
			field.Name = ""
			result.wildcard[v] = field
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

	wildcard := map[string]*structField{}
	for k, v := range t.wildcard {
		wildcard[k] = v
	}

	return &structType{
		typ:      t.typ,
		fields:   fields,
		wildcard: wildcard,
	}
}

// inherit populates the inherited fields. mapping is a map where the key is
// the inherited group key and the value is the name of the input to
// look for.
func (t *structType) inherit(mapping map[string]string) *structType {
	result := t.copy()
	for k, v := range mapping {
		if f, ok := result.wildcard[k]; ok {
			result.fields[v] = f
		}
	}

	return result
}

// assignableTo returns an assignability score for setting f onto f2.
// The higher the score, the better match it is. Anything result >= 0
// however can technically be assigned.
func (f *structField) assignableTo(f2 *structField) int {
	// If they're not assignable then they never match.
	if !f.Type.AssignableTo(f2.Type) {
		return -1
	}

	// If the names match, then we've got a slam dunk. Highest score.
	if f.Name == f2.Name {
		return 10
	}

	// If we're a wildcard, then we can be assigned to anything. But
	// we prefer to match names so give a lower score.
	if f.Name == "" || f2.Name == "" {
		return 5
	}

	// Assignable but neither is a wildcard.
	return -1
}

func (f *structField) String() string {
	return f.Name
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
