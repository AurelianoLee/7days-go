package server

import (
	"go/ast"
	"log"
	"reflect"
	"sync/atomic"
)

// 方法
type MethodType struct {
	method    reflect.Method // 方法本身
	ArgType   reflect.Type   // 第一个参数类型
	ReplyType reflect.Type   // 第二个参数类型
	numCalls  uint64         // 后续统计方法调用次数
}

func (m *MethodType) NumCalls() uint64 {
	// 用以原子操作的方式安全地读取值，避免了显示加锁的性能开销
	return atomic.LoadUint64(&m.numCalls)
}

func (m *MethodType) newArgv() reflect.Value {
	var argv reflect.Value
	// reflect.Elem() 获取一个指针类型的值所指向的具体类型
	// reflect.New() 创建一个指向该类型的指针 reflect.Value
	// arg may be a pointer type, or a value type
	if m.ArgType.Kind() == reflect.Pointer {
		// *int -> (*int).Elem() = int -> reflect.New(int) -> *int
		argv = reflect.New(m.ArgType.Elem())
	} else {
		// 根据这个类型创建一个指向该类型的值
		// 调用Elem()获取这个新创建值的非指针形式
		// int -> reflect.New(int) = *int -> (*int).Elem() = int
		argv = reflect.New(m.ArgType).Elem()
	}
	return argv
}

// newReplyv 用于为RPC方法的返回值创建一个合适的初始值
func (m *MethodType) newReplyv() reflect.Value {
	// reply must be a pointer type
	replyv := reflect.New(m.ReplyType.Elem())
	// 根据具体类型的初始化
	// 对于 map 和 slice 类型，直接使用 reflect.New() 创建的值未初始化的状态 nil map / nil slice
	// 使用 Elem().Set() 将新建的空值设置到 replyv 的底层值中
	switch m.ReplyType.Elem().Kind() {
	case reflect.Map:
		replyv.Elem().Set(reflect.MakeMap(m.ReplyType.Elem()))
	case reflect.Slice:
		replyv.Elem().Set(reflect.MakeSlice(m.ReplyType.Elem(), 0, 0))
	}
	return replyv
}

// 服务
type service struct {
	name   string                 // 映射的结构体的名称
	typ    reflect.Type           // 结构体的类型
	rcvr   reflect.Value          // 在调用时需要rcvr作为第0个参数
	method map[string]*MethodType // 存储映射的结构体的所有符合条件的方法
}

// newService 构造函数，根据入参的结构体实例创建对应的服务
func newService(rcvr any) *service {
	s := new(service)
	s.rcvr = reflect.ValueOf(rcvr)
	// reflect.Indirect() ->
	// 如果 rcvr 是一个指针类型，Indirect 返回该指针指向的值
	// 如果 rcvr 不是指针类型，则返回 rcvr 本身
	// Type() 返回这个类型的 reflect.Type
	// Name() 返回这个结构体类型的名字字符串
	s.name = reflect.Indirect(s.rcvr).Type().Name()
	s.typ = reflect.TypeOf(rcvr)
	if !ast.IsExported(s.name) {
		log.Fatalf("[RPC server]: %s is not a valid service name", s.name)
	}
	s.registerMethods()
	return s
}

// registerMethods 注册结构体中符合条件的方法
func (s *service) registerMethods() {
	s.method = make(map[string]*MethodType)
	for i := 0; i < s.typ.NumMethod(); i++ {
		method := s.typ.Method(i)
		mType := method.Type
		// 两个导出或内置类型的入参（反射时为3个，第0个是自身）
		// 返回值有且只有一个，且类型为 error
		if mType.NumIn() != 3 || mType.NumOut() != 1 {
			continue
		}
		if mType.Out(0) != reflect.TypeOf((*error)(nil)).Elem() {
			continue
		}
		argType, replyType := mType.In(1), mType.In(2)
		if !isExportedOrBuiltinType(argType) || !isExportedOrBuiltinType(replyType) {
			continue
		}
		s.method[method.Name] = &MethodType{
			method:    method,
			ArgType:   argType,
			ReplyType: replyType,
		}
		log.Printf("[RPC server]: register %s.%s\n", s.name, method.Name)
	}
}

// 检测这个类型是否是导出的类型或内建的类型
func isExportedOrBuiltinType(t reflect.Type) bool {
	return ast.IsExported(t.Name()) || t.PkgPath() == ""
}

func (s *service) call(m *MethodType, argv, replyv reflect.Value) error {
	atomic.AddUint64(&m.numCalls, 1)
	f := m.method.Func
	returnValues := f.Call([]reflect.Value{s.rcvr, argv, replyv})
	if errInter := returnValues[0].Interface(); errInter != nil {
		return errInter.(error)
	}
	return nil
}
