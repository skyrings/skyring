package salt_bindings

import (
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

	salt_wrapper = python.PyImport_ImportModuleNoBlock("salt_wrapper")
	if salt_wrapper == nil {
		panic("failed to import module salt_wrapper python module")
	}
}

func PyAddNode(node string, fingerprint string, username string, password string, master string) bool {
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
	py_out := add_node.CallObject(_a)

	return py_out.IsTrue()
}


func PyGetManagedNodes() map[string][]string {
	lock.Lock()
	defer lock.Unlock()

	_a := python.PyTuple_New(0)
	// call python function
	get_managed_nodes := salt_wrapper.GetAttrString("get_managed_nodes")
	if get_managed_nodes == nil {
		panic("failed to locate function get_managed_nodes")
	}
	py_out := get_managed_nodes.CallObject(_a)

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

	// TODO: have marshalling function to convert golang arguments to python argument
	// construct args
	_a := python.PyTuple_New(1)
	python.PyTuple_SET_ITEM(_a, 0, python.PyString_FromString(node))

	// call python function
	get_node_machine_id := salt_wrapper.GetAttrString("get_node_machine_id")
	if get_node_machine_id == nil {
		panic("failed to locate function get_node_machine_id")
	}
	py_out := get_node_machine_id.CallObject(_a)

	return python.PyString_AsString(py_out)
}


func PyGetNodeNetworkInfo(node string) map[string][]string {
	lock.Lock()
	defer lock.Unlock()

	// TODO: have marshalling function to convert golang arguments to python argument
	// construct args
	_a := python.PyTuple_New(1)
	python.PyTuple_SET_ITEM(_a, 0, python.PyString_FromString(node))

	// call python function
	get_node_network_info := salt_wrapper.GetAttrString("get_node_network_info")
	if get_node_network_info == nil {
		panic("failed to locate function get_node_network_info")
	}
	py_out := get_node_network_info.CallObject(_a)

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

	// TODO: have marshalling function to convert golang arguments to python argument
	// construct args
	_a := python.PyTuple_New(1)
	python.PyTuple_SET_ITEM(_a, 0, python.PyString_FromString(node))

	// call python function
	get_node_disk_info := salt_wrapper.GetAttrString("get_node_disk_info")
	if get_node_disk_info == nil {
		panic("failed to locate function get_node_disk_info")
	}
	py_out := get_node_disk_info.CallObject(_a)

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
