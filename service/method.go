package service

import (
	"fmt"
	"reflect"
	"runtime"
	"sync"
)

type EMethodType int
const (
	MethodTypeNone EMethodType = iota
	MethodTypeMethod
	MethodTypeFunction
)


type MethodType interface {
	GetMethodType() EMethodType
	GetArgType() reflect.Type
	GetReplyType() reflect.Type
	Call(in []reflect.Value) []reflect.Value
	FunName() string
}

type methodBase struct {
	sync.Mutex // protects counters
	ArgType    reflect.Type
	ReplyType  reflect.Type
	// numCalls   uint
}

func (mb *methodBase) GetArgType() reflect.Type {
	return mb.ArgType
}

func (mb *methodBase) GetReplyType() reflect.Type {
	return mb.ReplyType
}


type methodType struct {
	methodBase
	method     reflect.Method
}

type functionType struct {
	methodBase
	fn         reflect.Value
}

// 成员函数

func (mt *methodType) GetMethodType() EMethodType {
	return MethodTypeMethod
}

func (mt *methodType) Call(in []reflect.Value) []reflect.Value {
	function := mt.method.Func
	// Invoke the method, providing a new value for the reply.
	return function.Call(in)
}

func (mt *methodType) FunName() string {
	return mt.method.Name
}


// 普通函数

func (mf *functionType) GetMethodType() EMethodType {
	return MethodTypeFunction
}

func (mf *functionType) Call(in []reflect.Value) []reflect.Value {
	return mf.fn.Call(in)
}

func (mf *functionType) FunName() string {
	return fmt.Sprintf("%s", runtime.FuncForPC(mf.fn.Pointer()))
}