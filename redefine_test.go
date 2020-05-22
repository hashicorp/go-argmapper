package argmapper

import (
	"reflect"
	"strconv"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
)

func init() {
	hclog.L().SetLevel(hclog.Trace)
}

func TestFuncRedefine(t *testing.T) {
	cases := []struct {
		Name       string
		Func       interface{}
		Args       []Arg
		CallArgs   []Arg
		CallResult []interface{}
	}{
		{
			"all arguments satisfied",
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
			[]Arg{},
			[]interface{}{36},
		},

		{
			"missing a named argument",
			func(in struct {
				Struct

				A, B int
			}) int {
				return in.A + in.B
			},
			[]Arg{
				Named("a", 12),
			},
			[]Arg{
				Named("b", 24),
			},
			[]interface{}{36},
		},

		{
			"only through strings",
			func(in struct {
				Struct

				A, B int
			}) int {
				return in.A + in.B
			},
			[]Arg{
				Named("a", 12),
				WithConvFunc(func(v string) (int, error) { return strconv.Atoi(v) }),
				Filter(func(t reflect.Type) bool { return t.Kind() == reflect.String }),
			},
			[]Arg{
				Typed("24"),
			},
			[]interface{}{36},
		},
	}

	for _, tt := range cases {
		t.Run(tt.Name, func(t *testing.T) {
			require := require.New(t)

			f, err := NewFunc(tt.Func)
			require.NoError(err)

			redefined, err := f.Redefine(tt.Args...)
			require.NoError(err)

			result := redefined.Call(tt.CallArgs...)
			require.NoError(result.Err())
			for i, out := range tt.CallResult {
				require.Equal(out, result.Out(i))
			}
		})
	}
}
