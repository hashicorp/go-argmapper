package argmapper

type Arg func(*argBuilder)

type argBuilder struct {
	named map[string]interface{}
}

func Named(n string, v interface{}) Arg {
	return func(a *argBuilder) {
		a.named[n] = v
	}
}
