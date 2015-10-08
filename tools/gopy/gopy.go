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

func (f *PyFunction) Call(args ...interface{}) (r *python.PyObject, err error) {
	var pyargs *python.PyObject

	if pyargs, err = ToPyObject(reflect.ValueOf(args)); err != nil {
		return
	}

	name := python.PyString_AsString(f.GetAttrString("__name__"))
	mutex.Lock()
	defer mutex.Unlock()
	if r = f.CallObject(pyargs); r == nil {
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
	case reflect.Ptr:
		return convert(pyobj, reflect.Indirect(v))
	case reflect.Interface:
		return convert(pyobj, reflect.ValueOf(v.Interface()))
	case reflect.Bool:
		v.SetBool(pyobj.IsTrue())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if i64, overflow := python.PyLong_AsLongLongAndOverflow(pyobj); overflow == 0 {
			if i64 == -1 {
				fmt.Println("possibly exception occured at python side while converting PyLong")
			}
			if v.OverflowInt(i64) {
				err = errors.New(fmt.Sprintf("int overflow for Go value"))
			} else {
				v.SetInt(i64)
			}
		} else {
			err = errors.New(fmt.Sprintf("PyLong overflow"))
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if ui64 := python.PyLong_AsUnsignedLong(pyobj); v.OverflowUint(ui64) {
			err = errors.New(fmt.Sprintf("uint overflow for Go value"))
		} else {
			v.SetUint(ui64)
		}
	case reflect.Float32, reflect.Float64:
		// PyFloat_AsDouble() returns float32 only
		if f64 := float64(python.PyFloat_AsDouble(pyobj)); v.OverflowFloat(f64) {
			err = errors.New(fmt.Sprintf("float overflow for Go value"))
		} else {
			v.SetFloat(f64)
		}
	case reflect.String:
		v.SetString(python.PyString_AsString(pyobj))
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
	case reflect.Struct:
		t := v.Type()
		for i := 0; i < v.NumField(); i++ {
			if err = convert(python.PyDict_GetItemString(pyobj, t.Field(i).Name), v.Field(i)); err != nil {
				return
			}
		}
	default:
		panic(fmt.Sprintf("unsupported: %s type", v.Kind()))
	}
	return
}

func ToPyObject(v reflect.Value) (pyobj *python.PyObject, err error) {
	switch v.Kind() {
	case reflect.Ptr:
		return ToPyObject(reflect.Indirect(v))
	case reflect.Interface:
		return ToPyObject(reflect.ValueOf(v.Interface()))
	case reflect.Bool:
		if v.Bool() {
			pyobj = python.PyBool_FromLong(1)
		} else {
			pyobj = python.PyBool_FromLong(0)
		}
		if pyobj == nil {
			err = errors.New("nil PyObject for Go value bool")
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if pyobj = python.PyLong_FromLongLong(v.Int()); pyobj == nil {
			err = errors.New("nil PyObject for Go value int family")
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if pyobj = python.PyLong_FromUnsignedLongLong(v.Uint()); pyobj == nil {
			err = errors.New("nil PyObject for Go value uint family")
		}
	case reflect.Float32, reflect.Float64:
		f64 := v.Float()
		if f32 := float32(f64); float64(f32) != f64 {
			err = errors.New("float32 conversion loss in Go value")
		} else {
			// only float32 value is required
			if pyobj = python.PyFloat_FromDouble(f32); pyobj == nil {
				err = errors.New("nil PyObject for Go value float family")
			}
		}
	case reflect.String:
		if pyobj = python.PyString_FromString(v.String()); pyobj == nil {
			err = errors.New("nil PyObject for Go value string")
		}
	case reflect.Array:
		fallthrough
	case reflect.Slice:
		l := v.Len()
		if pyobj = python.PyTuple_New(l); pyobj == nil {
			err = errors.New(fmt.Sprintf("nil PyObject for Go value array/slice length %d", l))
		} else {
			var mempyobj *python.PyObject
			for i := 0; i < l; i++ {
				if mempyobj, err = ToPyObject(v.Index(i)); err != nil {
					pyobj = nil
					break
				}
				if err = python.PyTuple_SetItem(pyobj, i, mempyobj); err != nil {
					pyobj = nil
					break
				}
			}
		}
	case reflect.Map:
		if pyobj = python.PyDict_New(); pyobj == nil {
			err = errors.New("nil PyObject for Go value map")
		} else {
			var keypyobj, valuepyobj *python.PyObject
			for _, kv := range v.MapKeys() {
				if keypyobj, err = ToPyObject(kv); err != nil {
					pyobj = nil
					break
				}
				if valuepyobj, err = ToPyObject(v.MapIndex(kv)); err != nil {
					pyobj = nil
					break
				}
				if err = python.PyDict_SetItem(pyobj, keypyobj, valuepyobj); err != nil {
					pyobj = nil
					break
				}
			}
		}
	case reflect.Struct:
		if pyobj = python.PyDict_New(); pyobj == nil {
			err = errors.New("nil PyObject for Go value struct")
		} else {
			var keypyobj, valuepyobj *python.PyObject

			t := v.Type()
			for i := 0; i < v.NumField(); i++ {
				if keypyobj = python.PyString_FromString(t.Field(i).Name); keypyobj == nil {
					pyobj = nil
					err = errors.New("nil PyObject for Go value struct field name")
					break
				}
				if valuepyobj, err = ToPyObject(v.Field(i)); err != nil {
					pyobj = nil
					break
				}
				if err = python.PyDict_SetItem(pyobj, keypyobj, valuepyobj); err != nil {
					pyobj = nil
					break
				}
			}
		}
	default:
		panic(fmt.Sprintf("unsupported: %s type", v.Kind()))
	}
	return
}

func Bool(pyobj *python.PyObject) (b bool) {
	Convert(pyobj, &b)
	return
}
