package argmapper

import (
	"fmt"
	"reflect"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-multierror"
)

func (f *Func) Call(opts ...Arg) Result {
	// Build up our args
	state := &callState{
		Logger:   hclog.L(),
		Named:    map[string]reflect.Value{},
		Wildcard: map[*structField]reflect.Value{},
	}
	for _, opt := range opts {
		opt(state)
	}

	return f.call(state)
}

func (f *Func) call(cs *callState) Result {
	// Go through all our requirements and satisfy them.
	input := f.input
	for n, f := range input.fields {
		log := cs.Logger.With("field", f)
		log.Trace("attempting to satisfy field")
		v, ok := cs.Named[n]

		// If we found a value and its assignable then we've succeeded for this input.
		if ok {
			if v.Type().AssignableTo(f.Type) {
				log.Debug("field satisfied with exact value", "value", v.Interface())
				continue
			}

			// We have a value but it wasn't assignable. Set that as the
			// preferred inherit target so that we can get a conversion.
			cs.Wildcard[f] = v
		}

		// Okay we have a value but it wasn't assignable. Ideally we'd find
		// a converter that can convert our named input type to the target
		// type.
		ok, err := ConvSet(cs.Convs).provide(cs, f)
		if err != nil {
			log.Debug("error converting", "err", err)
			continue
		}
		if ok {
			log.Debug("field satisfied via conversion", "value", cs.Named[n].Interface())
		}
	}

	// If we have wildcards, we need to populate those with whatever we can.
	var mapping map[string]string
	if len(input.wildcard) > 0 {
		mapping = map[string]string{}

		// Satisfying our dynamic inputs is a bit different.
	WILDCARD:
		for n, f := range input.wildcard {
			for maybeField, maybeValue := range cs.Wildcard {
				if maybeField.Type.AssignableTo(f.Type) {
					mapping[n] = maybeField.Name
					cs.Named[maybeField.Name] = maybeValue
					continue WILDCARD
				}
			}

			for k, v := range cs.Named {
				if v.Type().AssignableTo(f.Type) {
					mapping[n] = k
					break
				}
			}
		}

		input = input.inherit(mapping)
	}

	// Go through and assign them
	var buildErr error
	structVal := input.New()
	for k := range input.fields {
		v, ok := cs.Named[k]
		if !ok {
			buildErr = multierror.Append(buildErr, fmt.Errorf(
				"argument cannot be satisfied: %s", k))
			continue
		}

		structVal.FieldNamed(k).Set(v)
	}

	// If we had any errors during the build, report those.
	if buildErr != nil {
		return Result{buildErr: buildErr}
	}

	// Call
	out := f.fn.Call([]reflect.Value{structVal.Value()})
	return Result{out: out, wildcardMapping: mapping}
}

type callState struct {
	Logger   hclog.Logger
	Named    map[string]reflect.Value
	Convs    []*Conv
	Wildcard map[*structField]reflect.Value
}
