package web

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"
)

func stableID(value string) string {
	return hex.EncodeToString([]byte(value))
}

var (
	errorType       = reflect.TypeOf((*error)(nil)).Elem()
	jsonMarshalerTy = reflect.TypeOf((*json.Marshaler)(nil)).Elem()
)

func completeJSONValue(v interface{}) interface{} {
	return completeJSONReflect(reflect.ValueOf(v))
}

func completeJSONReflect(v reflect.Value) interface{} {
	if !v.IsValid() {
		return nil
	}
	if v.Type().Implements(errorType) && (!canBeNil(v.Kind()) || !v.IsNil()) {
		err := v.Interface().(error)
		return map[string]interface{}{
			"message": err.Error(),
			"type":    fmt.Sprintf("%T", err),
		}
	}
	if v.Type().Implements(jsonMarshalerTy) && v.CanInterface() {
		return v.Interface()
	}
	if reflect.PointerTo(v.Type()).Implements(jsonMarshalerTy) && v.CanAddr() {
		return v.Addr().Interface()
	}
	switch v.Kind() {
	case reflect.Interface, reflect.Pointer:
		if v.IsNil() {
			return nil
		}
		return completeJSONReflect(v.Elem())
	case reflect.Struct:
		out := make(map[string]interface{}, v.NumField())
		t := v.Type()
		for i := 0; i < v.NumField(); i++ {
			field := t.Field(i)
			if field.PkgPath != "" {
				continue
			}
			name := jsonFieldName(field)
			if name == "-" {
				continue
			}
			out[name] = completeJSONReflect(v.Field(i))
		}
		return out
	case reflect.Map:
		if v.IsNil() {
			return nil
		}
		if v.Type().Key().Kind() != reflect.String {
			if v.CanInterface() {
				return v.Interface()
			}
			return fmt.Sprint(v)
		}
		out := make(map[string]interface{}, v.Len())
		iter := v.MapRange()
		for iter.Next() {
			out[iter.Key().String()] = completeJSONReflect(iter.Value())
		}
		return out
	case reflect.Slice, reflect.Array:
		if v.Kind() == reflect.Slice && v.IsNil() {
			return nil
		}
		out := make([]interface{}, v.Len())
		for i := 0; i < v.Len(); i++ {
			out[i] = completeJSONReflect(v.Index(i))
		}
		return out
	default:
		if v.CanInterface() {
			return v.Interface()
		}
		return fmt.Sprint(v)
	}
}

func jsonFieldName(field reflect.StructField) string {
	tag := field.Tag.Get("json")
	if tag == "" {
		return field.Name
	}
	name, _, _ := strings.Cut(tag, ",")
	if name == "" {
		return field.Name
	}
	return name
}

func canBeNil(kind reflect.Kind) bool {
	switch kind {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return true
	default:
		return false
	}
}

func newID() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}
