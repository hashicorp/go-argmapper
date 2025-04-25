// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package argmapper

import (
	"fmt"
	"reflect"

	"github.com/hashicorp/go-argmapper/internal/graph"
)

const (
	// weightNormal is the typcal edge weight.
	//nolint:unused
	weightNormal = 1

	// weightTyped is the weight to use for edges that connected to any
	// type-only vertex. We weigh these heavier since we prefer valued vertices.
	weightTyped = 5

	// weightTypedOtherSubtype is the weight to use for edges that connect
	// types that match but subtypes that do not match.
	weightTypedOtherSubtype = 20

	// weightMatchingName is the weight to use for the edges to any value
	// vertex with a matching name. This has the effect of preferring edges
	// from "A string" to "A int" for example (over "B string" to "A int"),
	// since we'd prefer to convert our original type.
	weightMatchingName = -1
)

// valueConverter is the interface implemented by vertices that can
// be represented by values. This is used to convert unexported vertex
// implementations into user-friendly information about what they represent.
type valueConverter interface {
	value() *Value
}

// valueVertex represents any named and typed value.
type valueVertex struct {
	Name    string
	Type    reflect.Type
	Subtype string
	Value   reflect.Value
}

func (v *valueVertex) Hashcode() interface{} {
	return fmt.Sprintf("%s/%s/%s", v.Name, v.Type.String(), v.Subtype)
}

// value returns the Value structures for this vertex. This is useful
// for error messages and other points where we must convert this to an
// exported, user-usable value.
func (v *valueVertex) value() *Value {
	return &Value{
		Name:    v.Name,
		Type:    v.Type,
		Subtype: v.Subtype,
		Value:   v.Value,
	}
}

// funcVertex is our target function. There is only ever one of these
// in the graph.
type funcVertex struct {
	Func *Func
}

func (v *funcVertex) Hashcode() interface{} { return v.Func.fn.Type() }
func (v *funcVertex) String() string        { return "func: " + v.Func.fn.String() }

// typedArgVertex represents a typed argument to a function. These have no
// name and match any matching types.
type typedArgVertex struct {
	Name    string
	Type    reflect.Type
	Subtype string

	Value reflect.Value
}

func (v *typedArgVertex) Hashcode() interface{} {
	return fmt.Sprintf("arg: %s/%s", v.Type.String(), v.Subtype)
}

func (v *typedArgVertex) String() string { return v.Hashcode().(string) }

// See valueVertex.value
func (v *typedArgVertex) value() *Value {
	return &Value{
		Name:    v.Name,
		Type:    v.Type,
		Subtype: v.Subtype,
		Value:   v.Value,
	}
}

// typedOutputVertex represents an output from a function that is typed
// only and has no name. This can be inherited by any value with a matching
// type.
type typedOutputVertex struct {
	Name    string
	Type    reflect.Type
	Subtype string

	Value reflect.Value
}

func (v *typedOutputVertex) Hashcode() interface{} {
	return fmt.Sprintf("out: %s/%s", v.Type.String(), v.Subtype)
}

func (v *typedOutputVertex) String() string {
	str := v.Hashcode().(string)
	if v.Value.IsValid() {
		str += fmt.Sprintf(" (value: %v)", v.Value.Interface())
	}

	return str
}

// See valueVertex.value
func (v *typedOutputVertex) value() *Value {
	return &Value{
		Name:    v.Name,
		Type:    v.Type,
		Subtype: v.Subtype,
		Value:   v.Value,
	}
}

// rootVertex tracks the root of a function call. This should have
// in-edges only from the inputs. We use this to get a single root.
type rootVertex struct{}

func (v *rootVertex) String() string { return "root" }

var (
	_ graph.VertexHashable = (*funcVertex)(nil)
	_ graph.VertexHashable = (*valueVertex)(nil)
	_ graph.VertexHashable = (*typedArgVertex)(nil)
	_ graph.VertexHashable = (*typedOutputVertex)(nil)
)
