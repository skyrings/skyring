package salt_bindings

import (
	"fmt"
	"github.com/sbinet/go-python"
	"sync"
)

var lock sync.Mutex
var salt_wrapper *python.PyObject

func init() {
	err := python.Initialize()
	if err != nil {
		panic(err.Error())
	}
}

func py_import(module_name string) *python.PyObject {
	_module := python.PyImport_ImportModuleNoBlock(module_name)
	if _module == nil {
		panic("failed to import module " + module_name)
	}
	return _module
}

func py_add_node(node string, fingerprint string, username string, password string, master string) bool {
	lock.Lock()
	defer lock.Unlock()

	// TODO: have marshalling function to convert golang arguments to python argument
	// construct args
	_a := python.PyTuple_New(5)
	python.PyTuple_SET_ITEM(_a, 0, python.PyString_FromString(node))
	python.PyTuple_SET_ITEM(_a, 1, python.PyString_FromString(fingerprint))
	python.PyTuple_SET_ITEM(_a, 2, python.PyString_FromString(username))
	python.PyTuple_SET_ITEM(_a, 3, python.PyString_FromString(password))
	python.PyTuple_SET_ITEM(_a, 4, python.PyString_FromString(master))

	// call python function
	add_node := salt_wrapper.GetAttrString("add_node")
	if add_node == nil {
		panic("failed to locate function add_node")
	}
	_result := add_node.CallObject(_a)

	return _result.IsTrue()
}


// func main() {
// 	salt_wrapper = py_import("salt_wrapper")
// 	fmt.Println(py_add_node("ceph-mon1.lab.eng.blr.redhat.com", "72:90:88:03:91:0d:3a:04:3b:e5:22:98:cb:34:c4:26", "root", "system", "usm-master.lab.eng.blr.redhat.com"))
// }
