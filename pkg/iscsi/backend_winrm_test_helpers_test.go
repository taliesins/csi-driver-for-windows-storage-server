package iscsi

import "reflect"

func copyTestOutput(dst, src any) {
	if dst == nil || src == nil {
		return
	}

	dstValue := reflect.ValueOf(dst)
	if dstValue.Kind() != reflect.Pointer || dstValue.IsNil() {
		return
	}
	dstElem := dstValue.Elem()
	srcValue := reflect.ValueOf(src)
	if srcValue.Kind() == reflect.Pointer {
		if srcValue.IsNil() {
			return
		}
		srcValue = srcValue.Elem()
	}

	switch {
	case dstElem.Kind() == reflect.Struct && srcValue.Kind() == reflect.Struct:
		copyStructFields(dstElem, srcValue)
	case dstElem.Kind() == reflect.Slice && srcValue.Kind() == reflect.Slice:
		copied := reflect.MakeSlice(dstElem.Type(), srcValue.Len(), srcValue.Len())
		for i := 0; i < srcValue.Len(); i++ {
			copyStructFields(copied.Index(i), srcValue.Index(i))
		}
		dstElem.Set(copied)
	case dstElem.Kind() == reflect.Map && srcValue.Kind() == reflect.Map:
		dstElem.Set(srcValue)
	}
}

func copyStructFields(dst, src reflect.Value) {
	for i := 0; i < src.NumField(); i++ {
		srcField := src.Type().Field(i)
		dstField := dst.FieldByName(srcField.Name)
		if dstField.IsValid() && dstField.CanSet() && src.Field(i).Type().AssignableTo(dstField.Type()) {
			dstField.Set(src.Field(i))
		}
	}
}
