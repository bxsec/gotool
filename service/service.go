package service

import (
	"context"
	"fmt"
	"reflect"
	"runtime"
	"unicode"
	"unicode/utf8"

	"github.com/bxsec/gotool/log"
)


// Precompute the reflect type for error. Can't use error directly
// because Typeof takes an empty interface value. This is annoying.
var typeOfError = reflect.TypeOf((*error)(nil)).Elem()

// Precompute the reflect type for context.
var typeOfContext = reflect.TypeOf((*context.Context)(nil)).Elem()



type Service struct {
	name     string                   // name of service
	rcvr     reflect.Value            // receiver of methods for the service
	typ      reflect.Type             // type of the receiver
	method   map[string]MethodType   // registered methods
}

func isExported(name string) bool {
	rune, _ := utf8.DecodeRuneInString(name)
	return unicode.IsUpper(rune)
}

func isExportedOrBuiltinType(t reflect.Type) bool {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	// PkgPath will be non-empty even for an exported type,
	// so we need to check the type name as well.
	return isExported(t.Name()) || t.PkgPath() == ""
}

func (s *Service) Call(ctx context.Context, method MethodType, argv, replyv reflect.Value) (err error) {
	defer func() {
		if r := recover(); r != nil {
			buf := make([]byte, 4096)
			n := runtime.Stack(buf, false)
			buf = buf[:n]

			err = fmt.Errorf("[service internal error]: %v, method: %s, argv: %+v, stack: %s",
				r, method.FunName(), argv.Interface(), buf)
			log.Error(err)
		}
	}()


	// Invoke the method, providing a new value for the reply.
	returnValues := method.Call([]reflect.Value{s.rcvr, reflect.ValueOf(ctx), argv, replyv})
	// The return value for the method is an error.
	errInter := returnValues[0].Interface()
	if errInter != nil {
		return errInter.(error)
	}

	return nil
}


//func (s *Service) Call2(ctx context.Context, mtype *methodType, argv, replyv reflect.Value) (err error) {
//	defer func() {
//		if r := recover(); r != nil {
//			buf := make([]byte, 4096)
//			n := runtime.Stack(buf, false)
//			buf = buf[:n]
//
//			err = fmt.Errorf("[service internal error]: %v, method: %s, argv: %+v, stack: %s",
//				r, mtype.method.Name, argv.Interface(), buf)
//			log.Error(err)
//		}
//	}()
//
//	function := mtype.method.Func
//	// Invoke the method, providing a new value for the reply.
//	returnValues := function.Call([]reflect.Value{s.rcvr, reflect.ValueOf(ctx), argv, replyv})
//	// The return value for the method is an error.
//	errInter := returnValues[0].Interface()
//	if errInter != nil {
//		return errInter.(error)
//	}
//
//	return nil
//}
//
//func (s *Service) CallForFunction(ctx context.Context, ft *functionType, argv, replyv reflect.Value) (err error) {
//	defer func() {
//		if r := recover(); r != nil {
//			buf := make([]byte, 4096)
//			n := runtime.Stack(buf, false)
//			buf = buf[:n]
//
//			// log.Errorf("failed to invoke service: %v, stacks: %s", r, string(debug.Stack()))
//			err = fmt.Errorf("[service internal error]: %v, function: %s, argv: %+v, stack: %s",
//				r, runtime.FuncForPC(ft.fn.Pointer()), argv.Interface(), buf)
//			log.Error(err)
//		}
//	}()
//
//	// Invoke the function, providing a new value for the reply.
//	returnValues := ft.fn.Call([]reflect.Value{reflect.ValueOf(ctx), argv, replyv})
//	// The return value for the method is an error.
//	errInter := returnValues[0].Interface()
//	if errInter != nil {
//		return errInter.(error)
//	}
//
//	return nil
//}
