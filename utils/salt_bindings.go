package util

import (
	"fmt"
	"github.com/sbinet/go-python"
	"reflect"
	"sync"
)

var lock sync.Mutex
var py_salt_wrapper *python.PyObject

var functions = [...]string{
	"get_node_ssh_fingerprint",
	"accept_node",
	"add_node",
	"get_nodes",
	"get_node_machine_id",
	"get_node_network_info",
	"get_node_disk_info",
	"execute_salt_commands_on_target",
}

type PyFunction struct {
	*python.PyObject
}

func (f *PyFunction) call(args ...interface{}) *python.PyObject {
	l := len(args)
	_a := python.PyTuple_New(l)

	for i := 0; i < l; i++ {
		python.PyTuple_SET_ITEM(_a, i, ToPyObject(reflect.ValueOf(args[i])))
	}

	func_name := python.PyString_AsString(f.GetAttrString("__name__"))

	lock.Lock()
	defer lock.Unlock()
	py_out := f.CallObject(_a)
	if py_out == nil {
		panic(fmt.Sprintf("function %s() failed at python side", func_name))
	}

	return py_out
}

var py_functions map[string]*PyFunction

func init() {
	var py_func *python.PyObject

	err := python.Initialize()
	if err != nil {
		panic(err.Error())
	}

	py_salt_wrapper = python.PyImport_ImportModuleNoBlock("skyring.salt_wrapper")
	if py_salt_wrapper == nil {
		panic("failed to import python module skyring.salt_wrapper")
	}

	py_functions = make(map[string]*PyFunction)
	for _, name := range functions {
		py_func = py_salt_wrapper.GetAttrString(name)
		if py_func == nil {
			panic(fmt.Sprintf("function %s() not found in python module skyring.salt_wrapper", name))
		}
		py_functions[name] = &PyFunction{py_func}
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
		// Too bad.  float64 is treated as long in python.
		// There will be chance of data loss to convert back to float32
		return python.PyFloat_FromDouble(float32(v.Float()))
	case reflect.Complex64, reflect.Complex128, reflect.Array, reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		panic("not supported")
	case reflect.String:
		return python.PyString_FromString(v.String())
	case reflect.Struct, reflect.UnsafePointer:
		panic("not supported")
	}
	return nil
}

func ToMapStringListString(py_dict *python.PyObject) map[string][]string {
	rv := make(map[string][]string)

	py_keys := python.PyDict_Keys(py_dict)
	keys_len := python.PyList_Size(py_keys)
	for i := 0; i < keys_len; i++ {
		key := python.PyString_AsString(python.PyList_GetItem(py_keys, i))
		py_list := python.PyDict_GetItemString(py_dict, key)
		len := python.PyList_Size(py_list)
		for i := 0; i < len; i++ {
			rv[key] = append(rv[key], python.PyString_AsString(python.PyList_GetItem(py_list, i)))
		}
	}

	return rv
}

func ToMapStringMapStringString(py_dict *python.PyObject) map[string]map[string]string {
	rv := make(map[string]map[string]string)
	py_keys := python.PyDict_Keys(py_dict)
	keys_len := python.PyList_Size(py_keys)
	for i := 0; i < keys_len; i++ {
		key := python.PyString_AsString(python.PyList_GetItem(py_keys, i))

		py_value_dict := python.PyDict_GetItemString(py_dict, key)
		py_vkeys := python.PyDict_Keys(py_value_dict)
		vkeys_len := python.PyList_Size(py_vkeys)

		dict := make(map[string]string)
		for i := 0; i < vkeys_len; i++ {
			vkey := python.PyString_AsString(python.PyList_GetItem(py_vkeys, i))
			dict[vkey] = python.PyString_AsString(python.PyDict_GetItemString(py_value_dict, vkey))
		}
		rv[key] = dict
	}

	return rv
}

func PyGetNodeSshFingerprint(node string) string {
	py_out := py_functions["get_node_ssh_fingerprint"].call(node)
	return python.PyString_AsString(py_out)
}

func PyAcceptNode(node string, fingerprint string) bool {
	py_out := py_functions["accept_node"].call(node, fingerprint)
	return py_out.IsTrue()
}

func PyAddNode(node string, fingerprint string, username string, password string, master string, port int) bool {
	py_out := py_functions["add_node"].call(node, fingerprint, username, password, master, port)
	return py_out.IsTrue()
}

func PyGetNodes() map[string]map[string]string {
	py_out := py_functions["get_nodes"].call()
	return ToMapStringMapStringString(py_out)
}

func PyGetNodeMachineId(node string) string {
	py_out := py_functions["get_node_machine_id"].call(node)
	return python.PyString_AsString(python.PyDict_GetItemString(py_out, node))
}

func PyGetNodeNetworkInfo(node string) map[string][]string {
	py_out := py_functions["get_node_network_info"].call(node)
	return ToMapStringListString(python.PyDict_GetItemString(py_out, node))
}

func PyExecuteSaltCommandsOnTarget(commands []string, target string) map[string][]string {
	py_out := py_functions["execute_salt_commands_on_target"].call(commands, target)
	return ToMapStringListString(py_out)
}

func PyGetNodeDiskInfo(node string) map[string]map[string]string {
	py_out := py_functions["get_node_disk_info"].call(node)
	return ToMapStringMapStringString(python.PyDict_GetItemString(py_out, node))
}
