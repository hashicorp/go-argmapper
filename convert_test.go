package argmapper

import (
	"reflect"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConvert(t *testing.T) {
	cases := []struct {
		Name     string
		Args     []Arg
		Target   interface{}
		Expected interface{}
	}{
		{
			"primitive to primitive",
			[]Arg{
				Typed("42"),
				Converter(func(v string) (int, error) { return strconv.Atoi(v) }),
			},
			(*int)(nil),
			int(42),
		},

		{
			"primitive to interface type",
			[]Arg{
				Typed("42"),
				Converter(func(v string) testInterface { return &testInterfaceImpl{} }),
			},
			(*testInterface)(nil),
			&testInterfaceImpl{},
		},

		{
			"primitive to interface implementation",
			[]Arg{
				Typed("42"),
				Converter(func(v string) *testInterfaceImpl { return &testInterfaceImpl{} }),
			},
			(*testInterface)(nil),
			&testInterfaceImpl{},
		},
	}

	for _, tt := range cases {
		t.Run(tt.Name, func(t *testing.T) {
			require := require.New(t)

			result, err := Convert(reflect.TypeOf(tt.Target).Elem(), tt.Args...)
			require.NoError(err)
			require.Equal(tt.Expected, result)
		})
	}
}

type testInterface interface {
	error
}

type testInterfaceImpl struct{}

func (*testInterfaceImpl) Error() string { return "hello" }
