package argmapper

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsStruct(t *testing.T) {
	cases := []struct {
		Name     string
		Test     interface{}
		Expected bool
	}{
		{
			"primitive",
			7,
			false,
		},

		{
			"struct embeds",
			struct {
				Struct
			}{},
			true,
		},
	}

	for _, tt := range cases {
		t.Run(tt.Name, func(t *testing.T) {
			require := require.New(t)

			actual := isStruct(reflect.TypeOf(tt.Test))
			require.Equal(tt.Expected, actual)
		})
	}
}
