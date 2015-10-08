package gopy

import (
	"errors"
	"fmt"
	"github.com/sbinet/go-python"
	"reflect"
	"sync"
)

type PyFunction struct {
	*python.PyObject
}

var mutex sync.Mutex

func (f *PyFunction) Call(params ...interface{}) (r *python.PyObject, err error) {
	l := len(params)
	args := python.PyTuple_New(l)

	for i := 0; i < l; i++ {
		python.PyTuple_SET_ITEM(args, i, ToPyObject(reflect.ValueOf(params[i])))
	}

	name := python.PyString_AsString(f.GetAttrString("__name__"))

	mutex.Lock()
	defer mutex.Unlock()
	if r = f.CallObject(args); r == nil {
		err = errors.New(fmt.Sprintf("%s(): function failed at python side", name))
	}
	return
}

var pyinit = false

func Init() (err error) {
	if !pyinit {
		if err = python.Initialize(); err == nil {
			pyinit = true
		}
	}
	return
}

func Import(module string, functions ...string) (funcs map[string]*PyFunction, err error) {
	if err = Init(); err != nil {
		return
	}

	if pymod := python.PyImport_ImportModuleNoBlock(module); pymod == nil {
		err = errors.New(fmt.Sprintf("gopy:%s: module import failed", module))
	} else {
		funcs = make(map[string]*PyFunction)
		for _, name := range functions {
			if pyfunc := pymod.GetAttrString(name); pyfunc == nil {
				err = errors.New(fmt.Sprintf("gopy:%s:%s: function not found", module, name))
				return
			} else {
				funcs[name] = &PyFunction{pyfunc}
			}
		}
	}
	return
}

func Convert(pyobj *python.PyObject, i interface{}) (err error) {
	rv := reflect.ValueOf(i)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		err = errors.New("invalid argument")
	} else {
		// rv is pointer type
		err = convert(pyobj, reflect.Indirect(rv))
	}
	return
}

func convert(pyobj *python.PyObject, v reflect.Value) (err error) {
	if pyobj == nil {
		err = errors.New("nil PyObject")
		return
	}

	switch v.Kind() {
	case reflect.Bool:
		v.SetBool(pyobj.IsTrue())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(int64(python.PyInt_AsLong(pyobj)))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v.SetUint(uint64(python.PyInt_AsUnsignedLongLongMask(pyobj)))
	case reflect.Float32, reflect.Float64:
		v.SetFloat(float64(python.PyFloat_AsDouble(pyobj)))
	case reflect.Array:
		len := python.PyList_Size(pyobj)
		if l := v.Len(); l != len {
			err = errors.New(fmt.Sprintf("array: length mismatch. expected %d, available %d", l, len))
			return
		}
		for i := 0; i < len; i++ {
			if err = convert(python.PyList_GetItem(pyobj, i), v.Index(i)); err != nil {
				return
			}
		}
	case reflect.Map:
		t := v.Type()
		keyval := reflect.Indirect(reflect.New(t.Key()))
		valval := reflect.Indirect(reflect.New(t.Elem()))

		pylist := python.PyDict_Items(pyobj)
		len := python.PyList_Size(pylist)
		for i := 0; i < len; i++ {
			pytup := python.PyList_GetItem(pylist, i)

			if err = convert(python.PyTuple_GetItem(pytup, 0), keyval); err != nil {
				return
			}

			if err = convert(python.PyTuple_GetItem(pytup, 1), valval); err != nil {
				return
			}

			v.SetMapIndex(keyval, valval)
		}
	case reflect.Ptr:
		return convert(pyobj, reflect.Indirect(v))
	case reflect.Slice:
		len := python.PyList_Size(pyobj)
		if c := v.Cap(); len > c {
			nv := reflect.MakeSlice(v.Type(), len, len)
			reflect.Copy(nv, v)
			v.Set(nv)
		}
		for i := 0; i < len; i++ {
			if err = convert(python.PyList_GetItem(pyobj, i), v.Index(i)); err != nil {
				return
			}
		}
	case reflect.String:
		v.SetString(python.PyString_AsString(pyobj))
	case reflect.Struct:
		t := v.Type()
		for i := 0; i < v.NumField(); i++ {
			if err = convert(python.PyDict_GetItemString(pyobj, t.Field(i).Name), v.Field(i)); err != nil {
				return
			}
		}
	case reflect.Uintptr:
		panic("unsupported: uintptr type")
	case reflect.Complex64, reflect.Complex128:
		panic("unsupported: complex family type")
	case reflect.Chan:
		panic("unsupported: channel type")
	case reflect.Func:
		panic("unsupported: func type")
	case reflect.Interface:
		panic("unsupported: interface type")
	case reflect.UnsafePointer:
		panic("unsupported: unsafepointer type")
	}
	return
}

func ToPyObject(v reflect.Value) *python.PyObject {
	switch v.Kind() {
	case reflect.Bool:
		if v.Bool() {
			return python.PyBool_FromLong(1)
		}
		return python.PyBool_FromLong(0)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return python.PyLong_FromLongLong(v.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return python.PyLong_FromUnsignedLongLong(v.Uint())
	case reflect.Float32, reflect.Float64:
		// Too bad.  float64 is treated as long in python.
		// A chance of data loss to convert back to float32
		return python.PyFloat_FromDouble(float32(v.Float()))
	case reflect.String:
		return python.PyString_FromString(v.String())
	case reflect.Uintptr:
		panic("unsupported: uintptr type")
	case reflect.Complex64, reflect.Complex128:
		panic("unsupported: complex family type")
	case reflect.Array:
		panic("unsupported: array type")
	case reflect.Chan:
		panic("unsupported: channel type")
	case reflect.Func:
		panic("unsupported: func type")
	case reflect.Interface:
		panic("unsupported: interface type")
	case reflect.Map:
		panic("unsupported: map type")
	case reflect.Ptr:
		panic("unsupported: ptr type")
	case reflect.Slice:
		panic("unsupported: slice type")
	case reflect.UnsafePointer:
		panic("unsupported: unsafepointer type")
	}
	return nil
}
