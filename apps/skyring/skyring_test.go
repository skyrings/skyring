package skyring

import (
	"encoding/json"
	"errors"
	"github.com/skyrings/skyring/conf"
	"github.com/skyrings/skyring/tools/logger"
	"net/rpc"
	"testing"
)

//Dummy File Path for Testing
var (
	configDir  = ""
	binDir     = ""
	ConfigFile = `{
    "config": {
        "host": "0.0.0.0",
        "httpPort": 8080,
        "supportedVersions": [
            1
        ]
    },
    "logging": {
        "logtostderr": false,
        "log_dir": "/abc/xyz/test",
        "v": 10,
        "vmodule": ""
    },
    "nodemanagementconfig": {
        "managerName": "testManager",
        "configFilePath": ""
    },
    "dbconfig": {
        "hostname": "112.5.0.1",
        "port": 24567,
        "database": "tesing",
        "user": "test",
        "password": "test"
    },
    "timeseriesdbconfig": {
        "hostname": "123.5.6.1",
        "port": 1234,
        "database": "collec",
        "user": "test",
        "password": "test"
    },
    "authentication": {
        "providerName": "abc",
        "configFile": "/xyz/test/test.conf"
    }
}`
	//ceph and gluster provider with valid information
	ProviderConf1 = []string{`{
    "provider": {
        "name": "ceph",
        "binary": "ceph_provider"
    },
    "routes": [
        {
            "name": "CephHello",
            "method": "GET",
            "pattern": "api/v1/ceph/hello/{name}",
            "pluginFunc": "SayHi",
            "version": 1
        }
    ]
}`,
		`{
    "provider": {
        "name": "gluster",
        "binary": "gluster_provider"
    },
    "routes": [
        {
            "name": "GlusterHello",
            "method": "GET",
            "pattern": "api/v1/gluster/hello/{name}",
            "pluginFunc": "SayHi",
            "version": 1
        }
    ]
}`,
	}
	//Empty provider list
	ProviderConf2 = []string{`{
  
}`,
	}
	//With Dupicate provider
	ProviderConf3 = []string{`{
    "provider": {
        "name": "ceph",
        "binary": "ceph_provider"
    },
    "routes": [
        {
            "name": "CephHello",
            "method": "GET",
            "pattern": "api/v1/ceph/hello/{name}",
            "pluginFunc": "SayHi",
            "version": 1
        }
    ]
}`,
		`{
    "provider": {
        "name": "ceph",
        "binary": "ceph_provider"
    },
    "routes": [
        {
            "name": "CephHello",
            "method": "GET",
            "pattern": "api/v1/ceph/hello/{name}",
            "pluginFunc": "SayHi",
            "version": 1
        }
    ]
}`,
	}
)

func Test_StartProviders(t *testing.T) {
	//Mock logger for avoiding Invalid address Error during Testing
	logger.MockInit()
	//Mock conf.Sysconfig structure for set Dummy configuration values
	MockLoadProviderConfig()
	var ProviderConf []string
	var err error
	//Mock LoadProviderConfig
	conf.LoadProviderConfig = func(providerConfigDir string) []conf.ProviderInfo {
		var (
			data       conf.ProviderInfo
			collection []conf.ProviderInfo
		)
		for _, f := range ProviderConf {

			var file = []byte(f)
			err = json.Unmarshal(file, &data)
			if err != nil {
				continue
			}
			collection = append(collection, data)
			data = conf.ProviderInfo{}
		}
		return collection
	}
	CallStartProviderCodec = func(path string, confStr []byte) (*rpc.Client, error) {
		var temp *rpc.Client
		return temp, err
	}
	test := &App{}
	err = nil
	ProviderConf = ProviderConf1
	test.providers = make(map[string]Provider)
	test.routes = make(map[string]conf.Route)
	//call actual function wants to Test
	if test.StartProviders(configDir, binDir); len(test.providers) < 1 {
		t.Error("None of the providers are initialized successfully")
	}
	//No providers found,ProviderConf2 contains no provider info
	test = &App{}
	err = nil
	ProviderConf = ProviderConf2
	test.providers = make(map[string]Provider)
	test.routes = make(map[string]conf.Route)
	//call actual function wants to Test
	if test.StartProviders(configDir, binDir); len(test.providers) > 0 {
		t.Error("None of the providers are initialized successfully")
	}
	//Chck duplication of provider is avoided
	test = &App{}
	err = errors.New("error")
	ProviderConf = ProviderConf3
	test.providers = make(map[string]Provider)
	test.routes = make(map[string]conf.Route)
	//call actual function wants to Test
	if test.StartProviders(configDir, binDir); len(test.providers) == 2 {
		t.Error("None of the providers are initialized successfully")
	}
}

func MockLoadProviderConfig() {
	file := []byte(ConfigFile)
	json.Unmarshal(file, &conf.SystemConfig)
}
