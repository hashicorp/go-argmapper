package argmapper

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
)

func init() {
	hclog.L().SetLevel(hclog.Trace)
}

func TestFunc(t *testing.T) {
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
			"argument cannot",
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
			"argument cannot",
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
			"cannot be",
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
