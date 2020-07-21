# go-argmapper [![Godoc](https://godoc.org/github.com/hashicorp/go-argmapper?status.svg)](https://godoc.org/github.com/hashicorp/go-argmapper)

go-argmapper is a dependency-injection library for Go that supports
automatically chaining conversion functions to reach desired results.
go-argmapper is designed for runtime, reflection-based dependency injection.

**API Status: Unstable.** We're still actively working on the API and
may change it in backwards incompatible ways. We don't think the API will
change significantly but it still can. 

## Features

**Named parameter matching.** go-argmapper can match on named arguments,
so you can say that `from int` is different from `to int` when calling
the same function.

**Typed parameter matching.** go-argmapper can match on types, including
interfaces and interface implementations. This enables the common
dependency-injection pattern of fulfilling an interface.

**"Subtype" labels for overloaded types.** Values can be labeled with a "subtype" key (a string)
for more fine-grained matching. A real-world use case of this is
[protobuf Any values](https://developers.google.com/protocol-buffers/docs/proto3#any).
The subtype of these values can be the protobuf message name. This enables
separating name, type, and subtype for more fine-grained matching.

**Automatic conversion function chaining.** You can configure multiple
"conversion functions" that can take some set of values and return another
set of values and go-argmapper will automatically call them in the correct
order if necessary to reach your desired function parameter types.

**Function redefinition in terms of certain types.** Functions can be
"redefined" to take as input and/or output values that match user-provided
filters. go-argmapper will automatically call proper conversion functions
to reach the target function.

**Type conversion API.** In addition to function calling, you can use the
automatic conversion function chaining to convert some input values to
any target value. go-argmapper will tell you (via an error) if this is not
possible.

## Examples

### Basic Dependency Injection

The example below shows common, basic dependency injection.

```go
// This is our target function. It wants some Writer implementation.
target, err := argmapper.NewFunc(func(w io.Writer) {
	// ... use the writer ...
})

// This is a provider that provides our io.Writer. You can imagine that
// this may differ between test/prod, configs, etc.
provider := func() io.Writer { return bytes.NewBuffer(nil) }

// Call our function. This will call our provider to create an io.Writer
// and then call our target function.
result := target.Call(argmapper.Converter(provider))
if result.Err() != nil {
	panic(result.Err())
}
```

The key thing happening here is that we're registering the `provider`
function as a "converter." argmapper will automatically find some converter
to provide any values we're looking for.

### Named and Typed Values

The example below shows both named and typed parameters in use.

```go
target, err := argmapper.NewFunc(func(input struct {
	// This tells argmapper to fill the values in this struct rather
	// than provide a value for the entire struct.
	argmapper.Struct

	A int
	B int
	Prefix string
}) string {
	return fmt.Sprintf("%s: %d", in.Prefix, in.A*in.B)
})

result := target.Call(
	argmapper.Named("a", 21),
	argmapper.Named("b", 2),
	argmapper.Typed("our value is"),
)
if result.Err() != nil {
	panic(result.Err())
}

// This prints: "our value is: 42"
println(result.Out(0).(string))
```

Both `A` and `B` are of the same type, but are matched on their names.
This lets us get the desired value of 42, rather than `21*21`, `2*2`, etc.

Note that `Prefix` is a named parameter, but we don't provide any
inputs matching that name. In this case, argmapper by default falls back
to treating it as a typed parameter, allowing our typed string input to
match.

### Explicitly Typed Values

The previous example showed `Prefix` implicitly using a typed-only
match since there was no input named "Prefix". You can also explictly
note that the name doesn't matter in two ways.

First, you can use struct tags:

```go
target, err := argmapper.NewFunc(func(input struct {
	// This tells argmapper to fill the values in this struct rather
	// than provide a value for the entire struct.
	argmapper.Struct

	A int
	B int
	Prefix string `argmapper:",typeOnly"`
}) string {
	return fmt.Sprintf("%s: %d", in.Prefix, in.A*in.B)
})
```

You can also use a non-struct input. Go reflection doesn't reveal
function parameter names so all function parameters are by definition
type only:

```go
target, err := argmapper.NewFunc(func(string) {})
```

You can mix and match named and typed parameters.

### Conversion Function Chaining

The example below shows how conversion functions are automatically
chained as necessary to reach your desired function.

```go
// Trivial function that takes a string and just returns it.
target, err := argmapper.NewFunc(func(v string) string { return v })

result := target.Call(
	// "false" value
	argmapper.Typed(false),

	// bool to int
	argmapper.Converter(func(v bool) int {
		if v {
			return 1
		}

		return 0
	}),

	// int to string
	argmapper.Converter(func(v int) string {
		return strconv.Itoa(v)
	}),
)
if result.Err() != nil {
	// If we didn't have converters necessary to get us from bool => int => string
	// then this would fail.
	panic(result.Err())
}

// Prints "0"
println(result.Out(0).(string))
```

Typed converters preserve the name of their arguments. If the above input
was `Named("foo", false)` rather than typed, then the name "foo" would
be attached both the string and int values generated in case any target
functions requested a named parameter. In the case of this example, the name
is carried through but carries no consequence since the final target
function is just a typed parameter.

### Conversion Function Cycles

Cycles in conversion functions are completely allowed. The example
below behaves as you would expect. This is a simple direct cycle, more complex
cycles from chaining multiple converters will also behave correctly. This
lets you register complex sets of bidirectional conversion functions with ease.

```go
// Trivial function that takes a string and just returns it.
target, err := argmapper.NewFunc(func(v string) string { return v })

result := target.Call(
	argmapper.Typed(12),
	argmapper.Converter(func(v int) string { return strconv.Itoa(v) }),
	argmapper.Converter(func(v string) (int, error) { return strconv.Atoi(v) }),
)
if result.Err() != nil {
	// If we didn't have converters necessary to get us from bool => int => string
	// then this would fail.
	panic(result.Err())
}

// Prints "12"
println(result.Out(0).(string))
```

### Conversion Errors

The example above has a converter that returns `(int, error)`. If the final
return type of a converter is `error`, go-argmapper treats that as a special
value signaling if the conversion succeeded or failed.

If conversion fails, the target function call fails and the error is returned
to the user.

In the future, we plan on retrying via other possible conversion paths
if they are available.
