package argmapper

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
)

func init() {
	hclog.L().SetLevel(hclog.Trace)
}

func TestFuncCall(t *testing.T) {
	// Used in some test cases.
	errSentinel := errors.New("error")

	// State used for some tests, reset each call
	var intState int

	cases := []struct {
		Name     string
		Callback interface{}
		Args     []Arg
		Out      []interface{}
		Err      string
	}{
		{
			"basic named matching",
			func(in struct {
				Struct

				A, B int
			}) int {
				return in.A + in.B
			},
			[]Arg{
				Named("a", 12),
				Named("b", 24),
			},
			[]interface{}{
				36,
			},
			"",
		},

		{
			"basic named matching (pointer struct)",
			func(in *struct {
				Struct

				A, B int
			}) int {
				return in.A + in.B
			},
			[]Arg{
				Named("a", 12),
				Named("b", 24),
			},
			[]interface{}{
				36,
			},
			"",
		},

		{
			"basic matching with types",
			func(in struct {
				Struct

				A int
			}) int {
				return in.A
			},
			[]Arg{
				Typed(42),
			},
			[]interface{}{
				42,
			},
			"",
		},

		{
			"basic matching with interface implementation",
			func(in struct {
				Struct

				A error
			}) (int, error) {
				return strconv.Atoi(in.A.Error())
			},
			[]Arg{
				Typed(fmt.Errorf("42")),
			},
			[]interface{}{
				42,
			},
			"",
		},

		{
			"basic matching with context",
			func(ctx context.Context) error { return ctx.Err() },
			[]Arg{
				Typed(context.Background()),
			},
			[]interface{}{},
			"",
		},

		{
			"missing argument",
			func(in struct {
				Struct

				A, B int
			}) int {
				return in.A + in.B
			},
			[]Arg{
				Named("a", 12),
			},
			nil,
			"could not be satisfied",
		},

		{
			"unexported field ignored",
			func(in struct {
				Struct

				A int
				b int
			}) int {
				return in.A
			},
			[]Arg{Named("a", 12)},
			[]interface{}{12},
			"",
		},

		{
			"no return values, only errors",
			func(in struct {
				Struct

				A, B int
			}) error {
				if in.A != 12 || in.B != 24 {
					// We panic to signal failure here instead of using
					// t but this should never happen.
					panic("failure")
				}

				return errSentinel
			},
			[]Arg{
				Named("a", 12),
				Named("b", 24),
			},
			[]interface{}{},
			errSentinel.Error(),
		},

		{
			"renamed field",
			func(in struct {
				Struct

				A int `argmapper:"C"`
				B int
			}) int {
				return in.A + in.B
			},
			[]Arg{
				Named("b", 24),
				Named("c", 12),
			},
			[]interface{}{
				36,
			},
			"",
		},

		{
			"typed and named prefers named",
			func(in struct {
				Struct

				A int
			}) int {
				return in.A
			},
			[]Arg{
				Named("a", 12),
				Typed(42),
			},
			[]interface{}{
				12,
			},
			"",
		},

		{
			"full struct matching",
			func(in struct {
				A int
				B int
			}) int {
				return in.A + in.B
			},
			[]Arg{
				Named("a", struct{ A, B int }{A: 12, B: 24}),
			},
			[]interface{}{
				36,
			},
			"",
		},

		//-----------------------------------------------------------
		// TYPED INPUT - STRUCT

		{
			"type only input",
			func(in struct {
				Struct

				A int `argmapper:",typeOnly"`
			}) int {
				return in.A
			},
			[]Arg{
				Named("b", 24),
			},
			[]interface{}{
				24,
			},
			"",
		},

		//-----------------------------------------------------------
		// TYPED INPUT - NON-STRUCT

		{
			"type only input non struct",
			func(v int) int { return v },
			[]Arg{
				Named("b", 24),
			},
			[]interface{}{
				24,
			},
			"",
		},

		//-----------------------------------------------------------
		// TYPE CONVERTER FUNCTIONS - NO STRUCT INPUTS

		{
			"type converter with no struct",
			func(in struct {
				Struct

				A string
				B int
			}) string {
				return strings.Repeat(in.A, in.B)
			},
			[]Arg{
				Named("a", 12),
				Named("b", 2),
				Converter(func(in int) string { return strconv.Itoa(in) }),
			},
			[]interface{}{"1212"},
			"",
		},

		{
			"type converter both ways",
			func(in struct {
				Struct

				A string
				B int
			}) string {
				return strings.Repeat(in.A, in.B)
			},
			[]Arg{
				Named("a", 12),
				Named("b", 2),
				Converter(func(in int) string { return strconv.Itoa(in) }),
				Converter(func(in string) (int, error) { return strconv.Atoi(in) }),
			},
			[]interface{}{"1212"},
			"",
		},

		{
			"type converter both ways, execute both",
			func(in struct {
				Struct

				A string
				B int
			}) string {
				return strings.Repeat(in.A, in.B)
			},
			[]Arg{
				Named("a", 12),
				Named("b", "2"),
				Converter(func(in int) string { return strconv.Itoa(in) }),
				Converter(func(in string) (int, error) { return strconv.Atoi(in) }),
			},
			[]interface{}{"1212"},
			"",
		},

		{
			"type converter with an error",
			func(in struct {
				Struct

				A string
			}) string {
				return in.A
			},
			[]Arg{
				Named("a", 12),
				Converter(func(in int) (string, error) {
					return "", fmt.Errorf("failed")
				}),
			},
			nil,
			"failed",
		},

		{
			"type converter with an error that succeeds",
			func(in struct {
				Struct

				A string
			}) string {
				return in.A
			},
			[]Arg{
				Named("a", 12),
				Converter(func(in int) (string, error) {
					return "YAY", nil
				}),
			},
			[]interface{}{"YAY"},
			"",
		},

		{
			"multiple input converter with no struct",
			func(in struct {
				Struct

				A int
			}) string {
				return strconv.Itoa(in.A)
			},
			[]Arg{
				Named("a", "foo"),
				Named("b", 2),
				Converter(func(v string, n int) int { return len(v) + n }),
			},
			[]interface{}{"5"},
			"",
		},

		{
			"chained converters",
			func(in struct {
				Struct

				A int
			}) string {
				return strconv.Itoa(in.A)
			},
			[]Arg{
				Named("input", 12),
				Converter(func(in int) string { return strconv.Itoa(in) }),
				Converter(func(in string) int { return len(in) }),
			},
			[]interface{}{"2"},
			"",
		},

		{
			"chained multiple input converter",
			func(in struct {
				Struct

				A int
				B string
			}) string {
				return in.B + " " + strconv.Itoa(in.A)
			},
			[]Arg{
				Typed(int8(42)),
				Typed(int16(1)),
				Converter(func(v int8) int { return int(v) }),
				Converter(func(v int, n int16) string { return "hello" }),
			},
			[]interface{}{"hello 42"},
			"",
		},

		{
			"no argument converter",
			func(in struct {
				Struct

				A int
				B string
			}) string {
				return in.B + strconv.Itoa(in.A)
			},
			[]Arg{
				Named("a", 12),
				Converter(func() string { return "yes: " }),
			},
			[]interface{}{"yes: 12"},
			"",
		},

		{
			"only providers",
			func(in struct {
				Struct

				A int
				B string
			}) string {
				return in.B + strconv.Itoa(in.A)
			},
			[]Arg{
				Converter(func() string { return "yes: " }),
				Converter(func() int { return 12 }),
			},
			[]interface{}{"yes: 12"},
			"",
		},

		{
			"multi-type providers",
			func(in struct {
				Struct

				A int
				B string
			}) string {
				return in.B + strconv.Itoa(in.A)
			},
			[]Arg{
				Converter(func() (string, int) { return "yes: ", 12 }),
			},
			[]interface{}{"yes: 12"},
			"",
		},

		{
			"multi-type converter with cycle",
			func(a int, b string) string {
				return b + strconv.Itoa(a)
			},
			[]Arg{
				Typed("no: "),
				Converter(func(int) (string, int) { return "yes: ", 12 }),
			},
			nil,
			"could not be satisfied",
		},

		{
			"unnecessary converters",
			func(ctx context.Context, v string) string { return v },
			[]Arg{
				Typed(context.Background()),
				Typed(42),
				Converter(func(v int) string { return strconv.Itoa(v) }),
				Converter(func(context.Context, []byte) string { return "" }),
				Converter(func(context.Context, string) []byte { return []byte("") }),
			},
			[]interface{}{"42"},
			"",
		},

		{
			"duplicate type converters",
			func(in struct {
				Struct

				A string
				B int
			}) string {
				return strings.Repeat(in.A, in.B)
			},
			[]Arg{
				Named("a", 12),
				Named("b", 2),
				Converter(func(in int) string { return strconv.Itoa(in) }),
				Converter(func(in int) string { return strconv.Itoa(in) }),
				Converter(func(in int) string { return strconv.Itoa(in) }),
				Converter(func(in int) string { return strconv.Itoa(in) }),
			},
			[]interface{}{"1212"},
			"",
		},

		//-----------------------------------------------------------
		// TYPE CONVERTER STRUCTS

		{
			"direct named converter",
			func(in struct {
				Struct

				A string
			}) string {
				return in.A + "!"
			},
			[]Arg{
				Named("a", 12),
				Converter(func(s struct {
					Struct

					A int
				}) struct {
					Struct

					A string
				} {
					return struct {
						Struct

						A string
					}{A: strconv.Itoa(s.A)}
				}),
			},
			[]interface{}{"12!"},
			"",
		},

		{
			"direct named converter (pointer)",
			func(in struct {
				Struct

				A string
			}) string {
				return in.A + "!"
			},
			[]Arg{
				Named("a", 12),
				Converter(func(s struct {
					Struct

					A int
				}) *struct {
					Struct

					A string
				} {
					return &struct {
						Struct

						A string
					}{A: strconv.Itoa(s.A)}
				}),
			},
			[]interface{}{"12!"},
			"",
		},

		{
			"direct named converter (pointer, nil result)",
			func(in struct {
				Struct

				A string
			}) string {
				return in.A + "!"
			},
			[]Arg{
				Named("a", 12),
				Converter(func(s struct {
					Struct

					A int
				}) *struct {
					Struct

					A string
				} {
					return nil
				}),
			},
			[]interface{}{"!"},
			"",
		},

		{
			"generic type converter",
			func(in struct {
				Struct

				A string
				B int
			}) string {
				return strings.Repeat(in.A, in.B)
			},
			[]Arg{
				Named("a", 12),
				Named("b", 2),
				Converter(func(s struct {
					Struct
					C string
				}) struct {
					Struct
					A string
				} {
					return struct {
						Struct
						A string
					}{A: "FOO"}
				}),
				Converter(func(s struct {
					Struct
					C bool
				}) struct {
					Struct
					A string
				} {
					return struct {
						Struct
						A string
					}{A: "FOO"}
				}),
				Converter(func(s struct {
					Struct
					B int `argmapper:",typeOnly"`
				}) struct {
					Struct
					B string `argmapper:",typeOnly"`
				} {
					return struct {
						Struct
						B string `argmapper:",typeOnly"`
					}{B: strconv.Itoa(s.B)}
				}),
			},
			[]interface{}{"1212"},
			"",
		},

		{
			"type converter with multiple typeOnly fields",
			func(in struct {
				Struct
				A string
				B int
			}) string {
				return strings.Repeat(in.A, in.B)
			},
			[]Arg{
				Named("a", 12),
				Named("b", "AB"),
				Converter(func(s struct {
					Struct
					C string `argmapper:",typeOnly"`
					D int    `argmapper:",typeOnly"`
				}) struct {
					Struct
					C int    `argmapper:",typeOnly"`
					D string `argmapper:",typeOnly"`
				} {
					return struct {
						Struct
						C int    `argmapper:",typeOnly"`
						D string `argmapper:",typeOnly"`
					}{C: len(s.C), D: strconv.Itoa(s.D)}
				}),
			},
			[]interface{}{"1212"},
			"",
		},

		//-----------------------------------------------------------
		// SUBTYPES

		{
			"subtype named match",
			func(in struct {
				Struct

				A int `argmapper:",subtype=foo"`
			}) int {
				return in.A
			},
			[]Arg{
				NamedSubtype("a", 24, "bar"),
				NamedSubtype("a", 36, "foo"),
			},
			[]interface{}{
				36,
			},
			"",
		},

		{
			"subtype no match",
			func(in struct {
				Struct

				A int `argmapper:",subtype=foo"`
			}) int {
				return in.A
			},
			[]Arg{
				NamedSubtype("a", 24, "bar"),
			},
			nil,
			"could not be",
		},

		{
			"subtype named conversion",
			func(in struct {
				Struct

				A int `argmapper:",subtype=foo"`
			}) int {
				return in.A
			},
			[]Arg{
				NamedSubtype("a", 24, "bar"),
				Converter(func(s struct {
					Struct

					A int `argmapper:",subtype=bar"`
				}) struct {
					Struct

					A int `argmapper:",subtype=foo"`
				} {
					return struct {
						Struct

						A int `argmapper:",subtype=foo"`
					}{A: s.A}
				}),
			},
			[]interface{}{
				24,
			},
			"",
		},

		{
			"subtype named not specified",
			func(in struct {
				Struct

				A int
			}) int {
				return in.A
			},
			[]Arg{
				NamedSubtype("a", 24, "bar"),
			},
			[]interface{}{
				24,
			},
			"",
		},

		{
			"subtype named not specified prefers exact match",
			func(in struct {
				Struct

				A int
			}) int {
				return in.A
			},
			[]Arg{
				Named("a", 24),
				NamedSubtype("a", 36, "bar"),
			},
			[]interface{}{
				24,
			},
			"",
		},

		{
			"subtype type match",
			func(in struct {
				Struct

				A int `argmapper:",typeOnly,subtype=foo"`
			}) int {
				return in.A
			},
			[]Arg{
				TypedSubtype(24, "bar"),
				TypedSubtype(36, "foo"),
			},
			[]interface{}{
				36,
			},
			"",
		},

		{
			"subtype type match named",
			func(in struct {
				Struct

				A int `argmapper:",typeOnly,subtype=foo"`
			}) int {
				return in.A
			},
			[]Arg{
				Named("A", 24),
				NamedSubtype("A", 24, "foo"),
			},
			[]interface{}{
				24,
			},
			"",
		},

		{
			"subtype type no match",
			func(in struct {
				Struct

				A int `argmapper:",typeOnly,subtype=foo"`
			}) int {
				return in.A
			},
			[]Arg{
				TypedSubtype(36, "bar"),
			},
			nil,
			"could not",
		},

		{
			"subtype type not specified",
			func(in struct {
				Struct

				A int `argmapper:",typeOnly"`
			}) int {
				return in.A
			},
			[]Arg{
				TypedSubtype(36, "foo"),
			},
			[]interface{}{
				36,
			},
			"",
		},

		{
			"subtype type conversion",
			func(in struct {
				Struct

				A int `argmapper:",typeOnly,subtype=foo"`
			}) int {
				return in.A
			},
			[]Arg{
				TypedSubtype(24, "bar"),
				Converter(func(s struct {
					Struct

					C int `argmapper:",typeOnly,subtype=bar"`
				}) struct {
					Struct

					D int `argmapper:",typeOnly,subtype=foo"`
				} {
					return struct {
						Struct

						D int `argmapper:",typeOnly,subtype=foo"`
					}{D: s.C + 1}
				}),
			},
			[]interface{}{
				25,
			},
			"",
		},

		{
			"subtype type conversion inherits subtype",
			func(in struct {
				Struct

				A int `argmapper:",typeOnly,subtype=foo"`
			}) int {
				return in.A
			},
			[]Arg{
				TypedSubtype("value", "bar"),
				Converter(func(s struct {
					Struct

					C string `argmapper:",typeOnly"`
				}) struct {
					Struct

					D int `argmapper:",typeOnly"`
				} {
					return struct {
						Struct

						D int `argmapper:",typeOnly"`
					}{D: 42}
				}),
			},
			[]interface{}{
				42,
			},
			"",
		},

		{
			"subtype type conversion generated",
			func(in struct {
				Struct

				A int `argmapper:",typeOnly,subtype=foo"`
			}) int {
				return in.A
			},
			[]Arg{
				TypedSubtype("foo", "bar"),
				ConverterGen(func(v Value) (*Func, error) {
					// We only want strings
					if v.Type != reflect.TypeOf("") {
						return nil, nil
					}

					// We take this value as our input.
					inputSet, err := NewValueSet([]Value{v})
					if err != nil {
						return nil, err
					}

					// Generate an int with the subtype of the string value
					outputSet, err := NewValueSet([]Value{Value{
						Name:    v.Name,
						Type:    reflect.TypeOf(int(0)),
						Subtype: v.Value.Interface().(string),
					}})
					if err != nil {
						return nil, err
					}

					return BuildFunc(inputSet, outputSet, func(in, out *ValueSet) error {
						outputSet.Typed(reflect.TypeOf(int(0))).Value =
							reflect.ValueOf(42)
						return nil
					})
				}),
			},
			[]interface{}{
				42,
			},
			"",
		},

		{
			"subtype type conversion generated multiple paths",
			func(in struct {
				Struct

				A int `argmapper:",typeOnly,subtype=foo"`
				B int `argmapper:",typeOnly,subtype=bar"`
			}) int {
				return in.A + in.B
			},
			[]Arg{
				TypedSubtype("foo", "12"),
				TypedSubtype("bar", "24"),
				ConverterGen(func(v Value) (*Func, error) {
					// We only want strings
					if v.Type != reflect.TypeOf("") {
						return nil, nil
					}

					// Convert subtype cause we're going to use it
					subtypeInt, err := strconv.Atoi(v.Subtype)
					if err != nil {
						return nil, err
					}

					// We take this value as our input.
					inputSet, err := NewValueSet([]Value{v})
					if err != nil {
						return nil, err
					}

					// Generate an int with the subtype of the string value
					outputSet, err := NewValueSet([]Value{Value{
						Name:    v.Name,
						Type:    reflect.TypeOf(int(0)),
						Subtype: v.Value.Interface().(string),
					}})
					if err != nil {
						return nil, err
					}

					return BuildFunc(inputSet, outputSet, func(in, out *ValueSet) error {
						outputSet.Typed(reflect.TypeOf(int(0))).Value =
							reflect.ValueOf(subtypeInt)
						return nil
					})
				}),
			},
			[]interface{}{
				36,
			},
			"",
		},

		{
			"subtype type matching named",
			func(in struct {
				Struct

				A int `argmapper:",typeOnly"`
			}) int {
				return in.A
			},
			[]Arg{
				NamedSubtype("b", 36, "bar"),
			},
			[]interface{}{
				36,
			},
			"",
		},

		{
			"empty struct arg",
			func(in struct {
				Struct

				A int `argmapper:",typeOnly,subtype=A"`
				B int `argmapper:",typeOnly,subtype=B"`
			}) error {
				return nil
			},
			[]Arg{
				// This empty struct arg was triggering a bug where this
				// wasn't resolving properly so this test exercises that in
				// an overly complex way.
				Converter(func(struct {
					Struct
				}) (struct {
					Struct

					X int `argmapper:",typeOnly,subType=A"`
				}, error) {
					return struct {
						Struct

						X int `argmapper:",typeOnly,subType=A"`
					}{X: 42}, nil
				}),

				Converter(func(struct {
					Struct

					X int `argmapper:",typeOnly,subType=A"`
				}) (struct {
					Struct

					Y int `argmapper:",typeOnly,subType=B"`
				}, error) {
					return struct {
						Struct

						Y int `argmapper:",typeOnly,subType=B"`
					}{Y: 84}, nil
				}),
			},
			nil,
			"",
		},

		//----------------------------------------------------------------
		// Once

		{
			"once option",
			func(in struct {
				Struct

				A     string
				B     bool
				State *int
			}) string {
				if !in.B {
					panic("OH NO")
				}

				return strconv.Itoa(*in.State)
			},
			[]Arg{
				Typed(&intState),
				Named("b", 2),

				ConverterFunc(MustFunc(NewFunc(func(b int, state *int) struct {
					Struct

					A int `argmapper:"A"`
				} {
					*state++

					return struct {
						Struct
						A int `argmapper:"A"`
					}{A: b * 2}
				}, FuncOnce()))),

				Converter(func(in struct {
					Struct

					A int `argmapper:"A"`
				}) string {
					return strconv.Itoa(in.A)
				}),

				Converter(func(in struct {
					Struct

					A int `argmapper:"A"`
				}) bool {
					return true
				}),
			},
			[]interface{}{"1"},
			"",
		},
	}

	for _, tt := range cases {
		t.Run(tt.Name, func(t *testing.T) {
			require := require.New(t)

			f, err := NewFunc(tt.Callback)
			require.NoError(err)
			result := f.Call(tt.Args...)

			// If we expect an error, check that
			if tt.Err == "" {
				require.NoError(result.Err())
			} else {
				require.Error(result.Err())
				t.Logf("err: %s", result.Err().Error())
				require.Contains(result.Err().Error(), tt.Err)
			}

			// Verify outputs
			require.Equal(len(tt.Out), result.Len())
			for i, out := range tt.Out {
				require.Equal(out, result.Out(i))
			}
		})
	}
}

// This tests a regression we found with argmapper after merging PR #9.
// This fails with PR #9 merged and passes without it. We added this test
// so that can revert PR #9 and safely merge a new fix when this also passes.
func TestFuncCall_waypointRepro(t *testing.T) {
	type aIn struct {
		Struct
		N int      `argmapper:",typeOnly"`
		C struct{} `argmapper:",typeOnly,subtype=C"`
		B struct{} `argmapper:",typeOnly,subtype=B"`
	}
	type aOut struct {
		Struct
		A struct{} `argmapper:",typeOnly,subtype=A"`
	}

	type bIn struct {
		Struct
		N int      `argmapper:",typeOnly"`
		C struct{} `argmapper:",typeOnly,subtype=C"`
	}
	type bOut struct {
		Struct
		B struct{} `argmapper:",typeOnly,subtype=B"`
	}

	type cIn struct {
		Struct
		N int `argmapper:",typeOnly"`
	}
	type cOut struct {
		Struct
		C struct{} `argmapper:",typeOnly,subtype=C"`
	}

	type targetIn struct {
		Struct

		// Note: this is the crux of the test: this ordering matters.
		C struct{} `argmapper:",typeOnly,subtype=C"`
		A struct{} `argmapper:",typeOnly,subtype=A"`
		B struct{} `argmapper:",typeOnly,subtype=B"`
	}

	a := MustFunc(NewFunc(func(aIn) (aOut, error) {
		return aOut{A: struct{}{}}, nil
	}))
	b := MustFunc(NewFunc(func(bIn) (bOut, error) {
		return bOut{B: struct{}{}}, nil
	}))
	c := MustFunc(NewFunc(func(cIn) (cOut, error) {
		return cOut{C: struct{}{}}, nil
	}))

	target := MustFunc(NewFunc(func(targetIn) error {
		return nil
	}))

	for i := 0; i < 100; i++ {
		result := target.Call(
			Typed(int(42)),
			ConverterFunc(c, a, b),
		)
		require.NoError(t, result.Err())
	}
}

func TestBuildFunc(t *testing.T) {
	require := require.New(t)

	intType := reflect.TypeOf(int(0))

	input, err := NewValueSet([]Value{
		Value{
			Name: "a",
			Type: intType,
		},
	})
	require.NoError(err)

	output, err := NewValueSet([]Value{
		Value{
			Type: intType,
		},
	})
	require.NoError(err)

	f, err := BuildFunc(input, output, func(in, out *ValueSet) error {
		// Double
		result := in.Named("a").Value.Interface().(int) * 2

		// Set the result
		out.Typed(intType).Value = reflect.ValueOf(result)

		return nil
	})
	require.NoError(err)

	require.NoError(output.FromResult(f.Call(Named("a", 12))))
	require.Equal(24, output.Typed(intType).Value.Interface())
}

// This tests a cycle found while working on the Waypoint project.
// We were using argmapper in a broken way so this should've never
// worked but it caused an infinite loop instead. This test originally
// infinite looped and remains to verify we don't regress.
func TestBuildFunc_waypointCycle(t *testing.T) {
	// This copies our behavior from Waypoint that triggered this bug.
	type markerType struct{}
	markerValue := func(n string) Value {
		val := markerType(struct{}{})
		return Value{
			Type:    reflect.TypeOf(val),
			Subtype: n,
			Value:   reflect.ValueOf(val),
		}
	}

	require := require.New(t)

	intType := reflect.TypeOf(int(0))
	stringType := reflect.TypeOf("")

	// Input is (int, string)
	input, err := NewValueSet([]Value{
		{
			Type: intType,
		},
		{
			Type: stringType,
		},
	})
	require.NoError(err)

	// Output is (marker<"A">, int)
	markerVal := markerValue("A")
	output, err := NewValueSet([]Value{
		markerVal,
		{
			Type: intType,
		},
	})
	require.NoError(err)

	// Our function does nothing
	f, err := BuildFunc(input, output, func(in, out *ValueSet) error {
		return nil
	})
	require.NoError(err)

	// Our target function just wants the marker value.
	input, err = NewValueSet([]Value{markerVal})
	require.NoError(err)
	target, err := BuildFunc(input, nil, func(in, out *ValueSet) error {
		return nil
	})
	require.NoError(err)

	// Input is the string and the converter.
	result := target.Call(
		Typed("hello"),
		ConverterFunc(f),
	)
	require.Error(result.Err())
}

func TestBuildFunc_noOutput(t *testing.T) {
	require := require.New(t)

	f, err := NewFunc(func(a int) {})
	require.NoError(err)

	f, err = BuildFunc(f.Input(), f.Output(), func(in, out *ValueSet) error {
		return nil
	})
	require.NoError(err)
	require.NoError(f.Output().FromResult(f.Call(Named("a", 12))))
}

func TestBuildFunc_nilOutput(t *testing.T) {
	require := require.New(t)

	f, err := NewFunc(func(a int) {})
	require.NoError(err)

	f, err = BuildFunc(f.Input(), nil, func(in, out *ValueSet) error {
		return nil
	})
	require.NoError(err)
	require.NoError(f.Output().FromResult(f.Call(Named("a", 12))))
}

func TestBuildFunc_nilInput(t *testing.T) {
	require := require.New(t)

	f, err := NewFunc(func(a *int) {})
	require.NoError(err)

	// Should not panic with a nil arg
	require.Error(f.Output().FromResult(f.Call(nil)))
}

func TestBuildFunc_errValue(t *testing.T) {
	require := require.New(t)

	intType := reflect.TypeOf(int(0))

	f, err := NewFunc(func(a int) {})
	require.NoError(err)

	input, err := NewValueSet([]Value{
		Value{
			Name: "a",
			Type: intType,
		},
	})
	require.NoError(err)

	f, err = BuildFunc(input, f.Output(), func(in, out *ValueSet) error {
		return fmt.Errorf("some error")
	})

	require.EqualError(f.Output().FromResult(f.Call(Named("a", 12))), "some error")
}

func TestFunc_defaultOpts(t *testing.T) {
	f, err := NewFunc(func(v int) string {
		return strconv.Itoa(v)
	}, Named("a", 42))
	require.NoError(t, err)

	result := f.Call(Typed("foo"))
	require.NoError(t, result.Err())
	require.Equal(t, "42", result.Out(0))
}
