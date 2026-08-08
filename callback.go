package dagro

import (
	"fmt"
	"reflect"
)

type optionalEdgeName struct{ value *string }

func isCallable(value any) bool {
	rv := reflect.ValueOf(value)
	return rv.IsValid() && rv.Kind() == reflect.Func
}

// callCallable adapts JavaScript-style callbacks to Go. JavaScript ignores
// surplus arguments and supplies undefined for missing ones; Go's zero value
// is the closest representable value for a missing statically typed argument.
func callCallable(fn any, args ...any) any {
	rv := reflect.ValueOf(fn)
	if !rv.IsValid() || rv.Kind() != reflect.Func {
		panic(fmt.Sprintf("dagro: expected callback, got %T", fn))
	}
	if rv.IsNil() {
		return nil
	}
	typ := rv.Type()
	if typ.NumOut() > 1 {
		panic(fmt.Sprintf("dagro: callback %T returns more than one value", fn))
	}

	fixed := typ.NumIn()
	if typ.IsVariadic() {
		fixed--
	}
	callArgs := make([]reflect.Value, 0, typ.NumIn()+len(args))
	for i := 0; i < fixed; i++ {
		if i < len(args) {
			callArgs = append(callArgs, callbackArgument(args[i], typ.In(i)))
		} else {
			callArgs = append(callArgs, reflect.Zero(typ.In(i)))
		}
	}
	if typ.IsVariadic() {
		elem := typ.In(typ.NumIn() - 1).Elem()
		for i := fixed; i < len(args); i++ {
			callArgs = append(callArgs, callbackArgument(args[i], elem))
		}
	}

	results := rv.Call(callArgs)
	if len(results) == 0 {
		return nil
	}
	return results[0].Interface()
}

func callbackArgument(value any, target reflect.Type) reflect.Value {
	if name, ok := value.(optionalEdgeName); ok {
		switch {
		case target.Kind() == reflect.Pointer && target.Elem().Kind() == reflect.String:
			if name.value == nil {
				return reflect.Zero(target)
			}
			return convertCallbackValue(reflect.ValueOf(name.value), target)
		case name.value == nil:
			return reflect.Zero(target)
		default:
			return convertCallbackValue(reflect.ValueOf(*name.value), target)
		}
	}
	if value == nil {
		return reflect.Zero(target)
	}
	return convertCallbackValue(reflect.ValueOf(value), target)
}

func convertCallbackValue(value reflect.Value, target reflect.Type) reflect.Value {
	if value.Type().AssignableTo(target) {
		return value
	}
	if value.Type().ConvertibleTo(target) {
		return value.Convert(target)
	}
	panic(fmt.Sprintf("dagro: callback argument %s is not assignable to %s", value.Type(), target))
}
