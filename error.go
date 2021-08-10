package argmapper

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/hashicorp/go-argmapper/internal/graph"
	"github.com/hashicorp/go-multierror"
)

// ErrArgumentUnsatisfied is the value returned when there is an argument
// to a target function that cannot be satisfied given the inputs and
// mappers.
type ErrArgumentUnsatisfied struct {
	// Func is the target function call that was attempted.
	Func *Func

	// Args are the args that aren't satisfied. Note that this won't have
	// the "Value" field set because an unsatisfied argument by definition
	// is missing a value.
	Args []*Value

	// Inputs is the list of values that were provided directly to the
	// function call that we could use to populate arguments.
	Inputs []*Value

	// Converters is the list of converter functions available for use.
	Converters []*Func
}

func (e *ErrArgumentUnsatisfied) Error() string {
	// Build our list of arguments the function expects
	fullArg := new(bytes.Buffer)
	for _, arg := range e.Func.Input().Values() {
		fmt.Fprintf(fullArg, "    - %s\n", arg.String())
	}

	// Build our list of missing arguments
	missing := new(bytes.Buffer)
	for _, arg := range e.Args {
		fmt.Fprintf(missing, "    - %s\n", arg.String())
	}

	// Build our list of missing arguments
	inputs := new(bytes.Buffer)
	if len(e.Inputs) == 0 {
		fmt.Fprintf(inputs, "    No inputs!\n")
	}
	for _, arg := range e.Inputs {
		fmt.Fprintf(inputs, "    - %s\n", arg.String())
	}

	convs := new(bytes.Buffer)
	if len(e.Converters) == 0 {
		fmt.Fprintf(convs, "    No converter functions.\n")
	}
	for _, conv := range e.Converters {
		fmt.Fprintf(convs, "    - %s\n", conv.Name())
		for _, arg := range conv.Input().Values() {
			fmt.Fprintf(convs, "        > %s\n", arg.String())
		}
		for _, arg := range conv.Output().Values() {
			fmt.Fprintf(convs, "        < %s\n", arg.String())
		}
	}

	return fmt.Sprintf(`
Argument to function %q could not be satisfied!

This means that one (or more) of the arguments to a function do not
have values that could be populated. A complete error description is below
for debugging.

==> Unsatisfiable arguments
    This is a list of the arguments that a value could not be found.

%s

==> Full list of desired function arguments
    This is a list of the arguments the function expected. Some arguments
    are named and some are unnamed. Unnamed arguments are matched by type.

%s

==> Full list of direct inputs
    This is a list of the direct inputs that were available. None of these
    matched the unsatisfied arguments. Note that inputs are also possible
    through mappers, listed after this section.

%s

==> Full list of available converters
    This is the list of functions that can be used to convert direct
    inputs (possibly being called in a chain) into a desired function
    argument. Arguments prefixed with ">" are inputs and arguments prefixed
    with "<" are outputs.

%s
`,
		e.Func.Name(),
		strings.TrimSuffix(missing.String(), "\n"),
		strings.TrimSuffix(fullArg.String(), "\n"),
		strings.TrimSuffix(inputs.String(), "\n"),
		strings.TrimSuffix(convs.String(), "\n"),
	)
}

// Updates the given error if it is an ErrArgumentUnsatisfied error
// with the input values and converter functions if they have not
// already been set.
func populateUnsatisfiedError(
	vertexI []graph.Vertex, // list of input vertices
	convs []*Func, // list of converter functions
	uErr error, // error to update
) error {
	var errs []error
	if merr, ok := uErr.(*multierror.Error); ok {
		errs = merr.Errors
	} else {
		errs = []error{uErr}
	}
	for _, errI := range errs {
		err, ok := errI.(*ErrArgumentUnsatisfied)
		if !ok {
			continue
		}

		if len(convs) > 0 && (err.Converters == nil || len(err.Converters) == 0) {
			err.Converters = convs
		}

		if len(vertexI) > 0 && (err.Inputs == nil || len(err.Inputs) == 0) {
			var inputs []*Value
			for _, v := range vertexI {
				valueable, ok := v.(valueConverter)
				if !ok {
					// This shouldn't be possible
					panic(fmt.Sprintf("argmapper graph node doesn't implement value(): %T", v))
				}

				inputs = append(inputs, valueable.value())
			}
			err.Inputs = inputs
		}
	}

	return uErr
}

var _ error = (*ErrArgumentUnsatisfied)(nil)
