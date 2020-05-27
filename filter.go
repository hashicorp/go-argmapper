package argmapper

import "reflect"

type FilterFunc func(Value) bool

// FilterType filters values based on matching the given type. If the type
// is an interface value then any types that implement the interface will
// also pass.
func FilterType(t reflect.Type) FilterFunc {
	return func(v Value) bool {
		// Direct match is always true
		if v.Type == t {
			return true
		}

		// If our type is an interface and the value type implements it, true
		return t.Kind() == reflect.Interface && v.Type.Implements(t)
	}
}

// FilterOr returns a FilterFunc that returns true if any of the given
// filter functions return true.
func FilterOr(fs ...FilterFunc) FilterFunc {
	return func(v Value) bool {
		for _, f := range fs {
			if f(v) {
				return true
			}
		}

		return false
	}
}

// FilterAnd returns a FilterFunc that returns true if any of the given
// filter functions return true.
func FilterAnd(fs ...FilterFunc) FilterFunc {
	return func(v Value) bool {
		for _, f := range fs {
			if !f(v) {
				return false
			}
		}

		return true
	}
}
