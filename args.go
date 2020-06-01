package argmapper

import (
	"reflect"
	"strings"

	"github.com/hashicorp/go-argmapper/internal/graph"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-multierror"
)

// Arg is an option to Func.Call that sets the state for the function call.
// This can be a direct named arg or a converter that could be used if
// necessary to reach the target.
type Arg func(*argBuilder) error

type argBuilder struct {
	logger   hclog.Logger
	named    map[string]reflect.Value
	namedSub map[string]map[string]reflect.Value
	typed    map[reflect.Type]reflect.Value
	typedSub map[reflect.Type]map[string]reflect.Value
	convs    []*Func
	convGens []ConverterGenFunc

	redefining   bool
	filterInput  FilterFunc
	filterOutput FilterFunc

	funcName string
}

func newArgBuilder(opts ...Arg) (*argBuilder, error) {
	builder := &argBuilder{
		logger:   hclog.L(),
		named:    make(map[string]reflect.Value),
		namedSub: make(map[string]map[string]reflect.Value),
		typed:    make(map[reflect.Type]reflect.Value),
		typedSub: make(map[reflect.Type]map[string]reflect.Value),
	}

	var buildErr error
	for _, opt := range opts {
		if err := opt(builder); err != nil {
			buildErr = multierror.Append(buildErr, err)
		}
	}

	return builder, buildErr
}

// Named specifies a named argument with the given value. This will satisfy
// any requirement where the name matches AND the value is assignable to
// the struct.
//
// If the name is an empty string, this is equivalent to calling Typed.
func Named(n string, v interface{}) Arg {
	if n == "" {
		return Typed(v)
	}

	return func(a *argBuilder) error {
		a.named[strings.ToLower(n)] = reflect.ValueOf(v)
		return nil
	}
}

// NamedSubtype is the same as Named but specifies a subtype for the value.
//
// If the name is an empty string, this is the equivalent to calling TypedSubtype.
func NamedSubtype(n string, v interface{}, st string) Arg {
	if n == "" {
		return TypedSubtype(v, st)
	}

	if st == "" {
		return Named(n, v)
	}

	return func(a *argBuilder) error {
		n = strings.ToLower(n)
		if a.namedSub[n] == nil {
			a.namedSub[n] = map[string]reflect.Value{}
		}
		a.namedSub[n][st] = reflect.ValueOf(v)
		return nil
	}
}

// Typed specifies a typed argument with the given value. This will satisfy
// any requirement where the type is assignable to a required value. The name
// can be anything of the required value.
func Typed(vs ...interface{}) Arg {
	return func(a *argBuilder) error {
		for _, v := range vs {
			rv := reflect.ValueOf(v)
			a.typed[rv.Type()] = rv
		}

		return nil
	}
}

// TypedSubtype is the same as Typed but specifies a subtype key for the value.
// If the subtype is empty, this is equivalent to calling Typed.
func TypedSubtype(v interface{}, st string) Arg {
	if st == "" {
		return Typed(v)
	}

	return func(a *argBuilder) error {
		rv := reflect.ValueOf(v)
		rt := rv.Type()
		if a.typedSub[rt] == nil {
			a.typedSub[rt] = map[string]reflect.Value{}
		}
		a.typedSub[rt][st] = rv
		return nil
	}
}

// Converter specifies one or more converters to use if necessary.
// A converter will be used if an argument type doesn't match exactly.
func Converter(fs ...interface{}) Arg {
	return func(a *argBuilder) error {
		for _, f := range fs {
			conv, err := NewFunc(f)
			if err != nil {
				return err
			}

			a.convs = append(a.convs, conv)
		}

		return nil
	}
}

// ConverterFunc is the same as Converter but takes an already created
// Func value. Any nil arguments are ignored. This appends to the list of
// converters.
func ConverterFunc(fs ...*Func) Arg {
	return func(a *argBuilder) error {
		for _, f := range fs {
			if f != nil {
				a.convs = append(a.convs, f)
			}
		}

		return nil
	}
}

// ConverterGenFunc is called with a value and should return a non-nil Func
// if it is able to generate a converter on the fly based on this value.
type ConverterGenFunc func(Value) (*Func, error)

// ConverterGen registers a converter generator. A converter generator
// generates a converter dynamically based on some set values. This can be
// used to generate type conversions for example. The returned func can
// have more requirements.
//
// If the function returns a nil Func, then no converter is generated.
func ConverterGen(fs ...ConverterGenFunc) Arg {
	return func(a *argBuilder) error {
		for _, f := range fs {
			a.convGens = append(a.convGens, f)
		}
		return nil
	}
}

// FilterInput is used by Func.Redefine to define what inputs are valid.
// This will replace any previously set FilterInput value. This has no effect
// unless Func.Redefine is being called.
func FilterInput(f FilterFunc) Arg {
	return func(a *argBuilder) error {
		a.filterInput = f
		return nil
	}
}

// FilterOutput is identical to FilterInput but for output values. If this
// is not set, then Redefine will allow any output values. This behavior is
// the same as if FilterInput were not specified.
func FilterOutput(f FilterFunc) Arg {
	return func(a *argBuilder) error {
		a.filterOutput = f
		return nil
	}
}

// Logger specifies a logger to be used during operations with these
// arguments. If this isn't specified, the default hclog.L() logger is used.
func Logger(l hclog.Logger) Arg {
	return func(a *argBuilder) error {
		a.logger = l
		return nil
	}
}

// FuncName sets the function name. This is used only with NewFunc.
func FuncName(n string) Arg {
	return func(a *argBuilder) error {
		a.funcName = n
		return nil
	}
}

func (b *argBuilder) graph(log hclog.Logger, g *graph.Graph, root graph.Vertex) []graph.Vertex {
	var result []graph.Vertex

	// Add our named inputs
	for k, v := range b.named {
		// Add the input
		input := g.AddOverwrite(&valueVertex{
			Name:  k,
			Type:  v.Type(),
			Value: v,
		})
		log.Trace("input", "kind", "named", "name", k, "type", v.Type(), "value", v)

		// Input depends on the input root
		g.AddEdge(input, root)

		// Track
		result = append(result, input)
	}

	// Add our named inputs with subtypes
	for k, m := range b.namedSub {
		for st, v := range m {
			// Add the input
			input := g.AddOverwrite(&valueVertex{
				Name:    k,
				Type:    v.Type(),
				Subtype: st,
				Value:   v,
			})
			log.Trace("input", "kind", "named", "name", k, "value", v, "subtype", st)

			// Input depends on the input root
			g.AddEdge(input, root)

			// Track
			result = append(result, input)
		}
	}

	// Add our typed inputs
	for t, v := range b.typed {
		// Add the input
		input := g.AddOverwrite(&typedOutputVertex{
			Type:  t,
			Value: v,
		})
		log.Trace("input", "kind", "typed", "type", t, "value", v)

		// Input depends on the input root
		g.AddEdge(input, root)

		// Track
		result = append(result, input)
	}

	// Add our typed inputs with subtypes
	for t, m := range b.typedSub {
		for st, v := range m {
			// Add the input
			input := g.AddOverwrite(&typedOutputVertex{
				Type:    t,
				Value:   v,
				Subtype: st,
			})
			log.Trace("input", "kind", "typed", "type", t, "value", v, "subtype", st)

			// Input depends on the input root
			g.AddEdge(input, root)

			// Track
			result = append(result, input)
		}
	}

	// If we have converters, add those.
	for _, f := range b.convs {
		f.graph(g, root, true)
	}

	// If we have converter generators, run those.
	if len(b.convGens) > 0 {
		for _, vertex := range g.Vertices() {
			// Get a value. If this vertex can't be represented by a value,
			// then ignore it.
			value := newValueFromVertex(vertex)
			if value == nil {
				continue
			}

			// Go through each generator and create the converter.
			for _, gen := range b.convGens {
				f, err := gen(*value)
				if err != nil {
					// TODO: return
					panic(err)
				}
				if f == nil {
					continue
				}

				f.graph(g, root, true)
			}
		}
	}
	return result
}
