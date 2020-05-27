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
