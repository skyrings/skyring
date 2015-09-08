package salt_bindings

import (
	"fmt"
	"github.com/sbinet/go-python"
	"sync"
	"reflect"
)

var lock sync.Mutex
var py_salt_wrapper *python.PyObject
var py_functions map[string]*python.PyObject

func init() {
	var py_func *python.PyObject

	err := python.Initialize()
	if err != nil {
		panic(err.Error())
	}

	py_salt_wrapper = python.PyImport_ImportModuleNoBlock("salt_wrapper")
	if py_salt_wrapper == nil {
		panic("failed to import python module salt_wrapper")
	}

	py_functions = make(map[string]*python.PyObject)
	for _, name := range []string{"add_node", "get_managed_nodes", "get_node_machine_id", "get_node_network_info", "get_node_disk_info"} {
		py_func = py_salt_wrapper.GetAttrString(name)
		if py_func == nil {
			panic(fmt.Sprintf("%s not found in python module salt_wrapper", name))
		}
		py_functions[name] = py_func
	}
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
	case reflect.Uintptr:
		panic("not supported")
	case reflect.Float32, reflect.Float64:
		return python.PyFloat_FromDouble(float32(v.Float()))
//	// Too bad.  float64 is treated as long in python
//	case reflect.Float64:
//		fmt.Println("float64 value:", v)
//		return python.PyLong_FromDouble(v.Float())
	case reflect.Complex64, reflect.Complex128, reflect.Array, reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		panic("not supported")
	case reflect.String:
		return python.PyString_FromString(v.String())
	case reflect.Struct, reflect.UnsafePointer:
		panic("not supported")
	}
	return nil
}

func PyArgs(args ...interface{}) *python.PyObject {
	l := len(args)
	_a := python.PyTuple_New(l)

	for i := 0; i < l; i++ {
		python.PyTuple_SET_ITEM(_a, i, ToPyObject(reflect.ValueOf(args[i])))
	}

	return _a
}

func PyAddNode(node string, fingerprint string, username string, password string, master string) bool {
	lock.Lock()
	defer lock.Unlock()

	_a := PyArgs(node, fingerprint, username, password, master)
	py_out := py_functions["add_node"].CallObject(_a)

	return py_out.IsTrue()
}

func PyGetManagedNodes() map[string][]string {
	lock.Lock()
	defer lock.Unlock()

	_a := PyArgs()
	py_out := py_functions["get_managed_nodes"].CallObject(_a)

	rv := make(map[string][]string)
	for _, key := range []string{"accepted_nodes", "denied_nodes", "unaccepted_nodes", "rejected_nodes"} {
		py_list := python.PyDict_GetItemString(py_out, key)
		len := python.PyList_Size(py_list)
		for i := 0; i < len; i++ {
			rv[key] = append(rv[key], python.PyString_AsString(python.PyList_GetItem(py_list, i)))
		}
	}

	return rv
}

func PyGetNodeMachineId(node string) string {
	lock.Lock()
	defer lock.Unlock()

	_a := PyArgs(node)
	py_out := py_functions["get_node_machine_id"].CallObject(_a)

	return python.PyString_AsString(py_out)
}

func PyGetNodeNetworkInfo(node string) map[string][]string {
	lock.Lock()
	defer lock.Unlock()

	_a := PyArgs(node)
	py_out := py_functions["get_node_network_info"].CallObject(_a)

	py_node_dict := python.PyDict_GetItemString(py_out, node)
	rv := make(map[string][]string)
	for _, key := range []string{"ipv4", "ipv6", "subnet"} {
		py_list := python.PyDict_GetItemString(py_node_dict, key)
		len := python.PyList_Size(py_list)
		for i := 0; i < len; i++ {
			rv[key] = append(rv[key], python.PyString_AsString(python.PyList_GetItem(py_list, i)))
		}
	}

	return rv
}

func PyGetNodeDiskInfo(node string) map[string]map[string]string {
	lock.Lock()
	defer lock.Unlock()

	_a := PyArgs(node)
	py_out := py_functions["get_node_disk_info"].CallObject(_a)

	py_node_dict := python.PyDict_GetItemString(py_out, node)
	rv := make(map[string]map[string]string)
	py_devnames := python.PyDict_Keys(py_node_dict)
	devnames_len := python.PyList_Size(py_devnames)
	for i := 0; i < devnames_len; i++ {
		devname := python.PyString_AsString(python.PyList_GetItem(py_devnames, i))

		py_devinfo := python.PyDict_GetItemString(py_node_dict, devname)
		py_keys := python.PyDict_Keys(py_devinfo)
		keys_len := python.PyList_Size(py_keys)

		devinfo := make(map[string]string)
		for i := 0; i < keys_len; i++ {
			key := python.PyString_AsString(python.PyList_GetItem(py_keys, i))
			devinfo[key] = python.PyString_AsString(python.PyDict_GetItemString(py_devinfo, key))
		}
		rv[devname] = devinfo
	}

	return rv
}
