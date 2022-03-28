package netx

import (
	"errors"
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"sync"

	"github.com/bxsec/gotool/log"
	rerrors "github.com/smallnest/rpcx/errors"
)

type ServiceManager struct {
	serviceMapMu sync.RWMutex
	serviceMap   map[string]*Service
}

// Register publishes in the server the set of methods of the
// receiver value that satisfy the following conditions:
//	- exported method of exported type
//	- three arguments, the first is of context.Context, both of exported type for three arguments
//	- the third argument is a pointer
//	- one return value, of type error
// It returns an error if the receiver is not an exported type or has
// no suitable methods. It also logs the error.
// The client accesses each method using a string of the form "Type.Method",
// where Type is the receiver's concrete type.
func (sm *ServiceManager) Register(rcvr interface{}, metadata string) error {
	sname, err := sm.register(rcvr, "", false)
	if err != nil {
		return err
	}
	return sm.Plugins.DoRegister(sname, rcvr, metadata)
}

// RegisterName is like Register but uses the provided name for the type
// instead of the receiver's concrete type.
func (sm *ServiceManager) RegisterName(name string, rcvr interface{}, metadata string) error {
	_, err := sm.register(rcvr, name, true)
	if err != nil {
		return err
	}
	if sm.Plugins == nil {
		sm.Plugins = &pluginContainer{}
	}
	return sm.Plugins.DoRegister(name, rcvr, metadata)
}

// RegisterFunction publishes a function that satisfy the following conditions:
//	- three arguments, the first is of context.Context, both of exported type for three arguments
//	- the third argument is a pointer
//	- one return value, of type error
// The client accesses function using a string of the form "servicePath.Method".
func (sm *ServiceManager) RegisterFunction(servicePath string, fn interface{}, metadata string) error {
	fname, err := sm.registerFunction(servicePath, fn, "", false)
	if err != nil {
		return err
	}

	return sm.Plugins.DoRegisterFunction(servicePath, fname, fn, metadata)
}

// RegisterFunctionName is like RegisterFunction but uses the provided name for the function
// instead of the function's concrete type.
func (sm *ServiceManager) RegisterFunctionName(servicePath string, name string, fn interface{}, metadata string) error {
	_, err := sm.registerFunction(servicePath, fn, name, true)
	if err != nil {
		return err
	}

	return sm.Plugins.DoRegisterFunction(servicePath, name, fn, metadata)
}

func (sm *ServiceManager) register(rcvr interface{}, name string, useName bool) (string, error) {
	sm.serviceMapMu.Lock()
	defer sm.serviceMapMu.Unlock()

	service := new(Service)
	service.typ = reflect.TypeOf(rcvr)
	service.rcvr = reflect.ValueOf(rcvr)
	sname := reflect.Indirect(service.rcvr).Type().Name() // Type
	if useName {
		sname = name
	}
	if sname == "" {
		errorStr := "rpcx.Register: no service name for type " + service.typ.String()
		log.Error(errorStr)
		return sname, errors.New(errorStr)
	}
	if !useName && !isExported(sname) {
		errorStr := "rpcx.Register: type " + sname + " is not exported"
		log.Error(errorStr)
		return sname, errors.New(errorStr)
	}
	service.name = sname

	// Install the methods
	service.method = suitableMethods(service.typ, true)

	if len(service.method) == 0 {
		var errorStr string

		// To help the user, see if a pointer receiver would work.
		method := suitableMethods(reflect.PtrTo(service.typ), false)
		if len(method) != 0 {
			errorStr = "rpcx.Register: type " + sname + " has no exported methods of suitable type (hint: pass a pointer to value of that type)"
		} else {
			errorStr = "rpcx.Register: type " + sname + " has no exported methods of suitable type"
		}
		log.Error(errorStr)
		return sname, errors.New(errorStr)
	}
	sm.serviceMap[service.name] = service
	return sname, nil
}

func (sm *ServiceManager) registerFunction(servicePath string, fn interface{}, name string, useName bool) (string, error) {
	sm.serviceMapMu.Lock()
	defer sm.serviceMapMu.Unlock()

	ss := sm.serviceMap[servicePath]
	if ss == nil {
		ss = new(Service)
		ss.name = servicePath
		ss.function = make(map[string]*functionType)
	}

	f, ok := fn.(reflect.Value)
	if !ok {
		f = reflect.ValueOf(fn)
	}
	if f.Kind() != reflect.Func {
		return "", errors.New("function must be func or bound method")
	}

	fname := runtime.FuncForPC(reflect.Indirect(f).Pointer()).Name()
	if fname != "" {
		i := strings.LastIndex(fname, ".")
		if i >= 0 {
			fname = fname[i+1:]
		}
	}
	if useName {
		fname = name
	}
	if fname == "" {
		errorStr := "rpcx.registerFunction: no func name for type " + f.Type().String()
		log.Error(errorStr)
		return fname, errors.New(errorStr)
	}

	t := f.Type()
	if t.NumIn() != 3 {
		return fname, fmt.Errorf("rpcx.registerFunction: has wrong number of ins: %s", f.Type().String())
	}
	if t.NumOut() != 1 {
		return fname, fmt.Errorf("rpcx.registerFunction: has wrong number of outs: %s", f.Type().String())
	}

	// First arg must be context.Context
	ctxType := t.In(0)
	if !ctxType.Implements(typeOfContext) {
		return fname, fmt.Errorf("function %s must use context as  the first parameter", f.Type().String())
	}

	argType := t.In(1)
	if !isExportedOrBuiltinType(argType) {
		return fname, fmt.Errorf("function %s parameter type not exported: %v", f.Type().String(), argType)
	}

	replyType := t.In(2)
	if replyType.Kind() != reflect.Ptr {
		return fname, fmt.Errorf("function %s reply type not a pointer: %s", f.Type().String(), replyType)
	}
	if !isExportedOrBuiltinType(replyType) {
		return fname, fmt.Errorf("function %s reply type not exported: %v", f.Type().String(), replyType)
	}

	// The return type of the method must be error.
	if returnType := t.Out(0); returnType != typeOfError {
		return fname, fmt.Errorf("function %s returns %s, not error", f.Type().String(), returnType.String())
	}

	// Install the methods
	ss.function[fname] = &functionType{fn: f, ArgType: argType, ReplyType: replyType}
	sm.serviceMap[servicePath] = ss

	// init pool for reflect.Type of args and reply
	reflectTypePools.Init(argType)
	reflectTypePools.Init(replyType)

	return fname, nil
}

// suitableMethods returns suitable Rpc methods of typ, it will report
// error using log if reportErr is true.
func suitableMethods(typ reflect.Type, reportErr bool) map[string]*methodType {
	methods := make(map[string]*methodType)
	for m := 0; m < typ.NumMethod(); m++ {
		method := typ.Method(m)
		mtype := method.Type
		mname := method.Name
		// Method must be exported.
		if method.PkgPath != "" {
			continue
		}
		// Method needs four ins: receiver, context.Context, *args, *reply.
		if mtype.NumIn() != 4 {
			if reportErr {
				log.Debug("method ", mname, " has wrong number of ins:", mtype.NumIn())
			}
			continue
		}
		// First arg must be context.Context
		ctxType := mtype.In(1)
		if !ctxType.Implements(typeOfContext) {
			if reportErr {
				log.Debug("method ", mname, " must use context.Context as the first parameter")
			}
			continue
		}

		// Second arg need not be a pointer.
		argType := mtype.In(2)
		if !isExportedOrBuiltinType(argType) {
			if reportErr {
				log.Info(mname, " parameter type not exported: ", argType)
			}
			continue
		}
		// Third arg must be a pointer.
		replyType := mtype.In(3)
		if replyType.Kind() != reflect.Ptr {
			if reportErr {
				log.Info("method", mname, " reply type not a pointer:", replyType)
			}
			continue
		}
		// Reply type must be exported.
		if !isExportedOrBuiltinType(replyType) {
			if reportErr {
				log.Info("method", mname, " reply type not exported:", replyType)
			}
			continue
		}
		// Method needs one out.
		if mtype.NumOut() != 1 {
			if reportErr {
				log.Info("method", mname, " has wrong number of outs:", mtype.NumOut())
			}
			continue
		}
		// The return type of the method must be error.
		if returnType := mtype.Out(0); returnType != typeOfError {
			if reportErr {
				log.Info("method", mname, " returns ", returnType.String(), " not error")
			}
			continue
		}
		methods[mname] = &methodType{method: method, ArgType: argType, ReplyType: replyType}

		// init pool for reflect.Type of args and reply
		reflectTypePools.Init(argType)
		reflectTypePools.Init(replyType)
	}
	return methods
}

// UnregisterAll unregisters all services.
// You can call this method when you want to shutdown/upgrade this node.
func (sm *ServiceManager) UnregisterAll() error {
	sm.serviceMapMu.RLock()
	defer sm.serviceMapMu.RUnlock()
	var es []error
	for k := range sm.serviceMap {
		err := sm.Plugins.DoUnregister(k)
		if err != nil {
			es = append(es, err)
		}
	}

	if len(es) > 0 {
		return rerrors.NewMultiError(es)
	}
	return nil
}
