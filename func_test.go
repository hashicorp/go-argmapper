package argmapper

import (
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
			"basic matching",
			func(in struct {
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
			"missing argument",
			func(in struct {
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
			"direct named converter",
			func(in struct {
				A string
			}) string {
				return in.A + "!"
			},
			[]Arg{
				Named("a", 12),
				WithConvFunc(func(s struct {
					A int
				}) struct {
					A string
				} {
					return struct{ A string }{strconv.Itoa(s.A)}
				}),
			},
			[]interface{}{"12!"},
			"",
		},

		{
			"type converter with no struct",
			func(in struct {
				A string
				B int
			}) string {
				return strings.Repeat(in.A, in.B)
			},
			[]Arg{
				Named("a", 12),
				Named("b", 2),
				WithConvFunc(func(in int) string { return strconv.Itoa(in) }),
			},
			[]interface{}{"1212"},
			"",
		},

		{
			"multiple input converter with no struct",
			func(in struct {
				A int
			}) string {
				return strconv.Itoa(in.A)
			},
			[]Arg{
				Named("a", "foo"),
				Named("b", 2),
				WithConvFunc(func(v string, n int) int { return len(v) + n }),
			},
			[]interface{}{"5"},
			"",
		},

		{
			"chained converters",
			func(in struct {
				A int
			}) string {
				return strconv.Itoa(in.A)
			},
			[]Arg{
				Named("input", 12),
				WithConvFunc(func(in int) string { return strconv.Itoa(in) }),
				WithConvFunc(func(in string) int { return len(in) }),
			},
			[]interface{}{"2"},
			"",
		},

		{
			"no argument converter",
			func(in struct {
				A int
				B string
			}) string {
				return in.B + strconv.Itoa(in.A)
			},
			[]Arg{
				Named("a", 12),
				WithConvFunc(func() string { return "yes: " }),
			},
			[]interface{}{"yes: 12"},
			"",
		},

		{
			"generic type converter",
			func(in struct {
				A string
				B int
			}) string {
				return strings.Repeat(in.A, in.B)
			},
			[]Arg{
				Named("a", 12),
				Named("b", 2),
				WithConvFunc(func(s struct {
					C string
				}) struct {
					A string
				} {
					return struct {
						A string
					}{"FOO"}
				}),
				WithConvFunc(func(s struct {
					C bool
				}) struct {
					A string
				} {
					return struct {
						A string
					}{"FOO"}
				}),
				WithConvFunc(func(s struct {
					B int `argmapper:",typeOnly"`
				}) struct {
					B string `argmapper:",typeOnly"`
				} {
					return struct {
						B string `argmapper:",typeOnly"`
					}{strconv.Itoa(s.B)}
				}),
			},
			[]interface{}{"1212"},
			"",
		},

		{
			"type converter with multiple typeOnly fields",
			func(in struct {
				A string
				B int
			}) string {
				return strings.Repeat(in.A, in.B)
			},
			[]Arg{
				Named("a", 12),
				Named("b", "AB"),
				WithConvFunc(func(s struct {
					C string `argmapper:",typeOnly"`
					D int    `argmapper:",typeOnly"`
				}) struct {
					C int    `argmapper:",typeOnly"`
					D string `argmapper:",typeOnly"`
				} {
					return struct {
						C int    `argmapper:",typeOnly"`
						D string `argmapper:",typeOnly"`
					}{len(s.C), strconv.Itoa(s.D)}
				}),
			},
			[]interface{}{"1212"},
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
