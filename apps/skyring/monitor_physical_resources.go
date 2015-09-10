/*Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package skyring

import (
    "encoding/json"
    "fmt"
    "net/url"
    "net/http"
    "github.com/golang/glog"
    "github.com/gorilla/mux"

    influxdb "github.com/influxdb/influxdb/client"
)

const (
    DBHost = "influxdb"
    DBPort = 8086
    DBName = "collectd"
    DBUSer = "admin"
    DBPasswd = "admin"
)

func queryDB(con *influxdb.Client, cmd string) (res []influxdb.Result, err error) {
    q := influxdb.Query {
        Command: cmd,
        Database: DBName,
    }

    if response, err := con.Query(q); err == nil {
        if response.Error() != nil {
            return res, response.Error()
        }
        res = response.Results
    }
    return
}

func CpuUtilizationHandler(w http.ResponseWriter, r *http.Request) {
    u, err := url.Parse(fmt.Sprintf("http://%s:%d", DBHost, DBPort))
    if err != nil {
        glog.Fatalf("Error: ", err)
    }

    client, err := influxdb.NewClient(influxdb.Config {
        URL: *u,
        Username: DBUSer,
        Password: DBPasswd,
    })
    if err != nil {
        glog.Fatalf("Error: ", err)
    }

    vars := mux.Vars(r)
    host_name := vars["host-name"]

    query_cmd := fmt.Sprintf("SELECT * FROM /(%s.cpu).*/", host_name)
    res, err := queryDB(client, query_cmd)
    if err == nil {
        json.NewEncoder(w).Encode(res[0].Series)
    } else {
        w.Header().Set("Content-Type", "application/json;charset=UTF-8")
        w.WriteHeader(422)
        if err := json.NewEncoder(w).Encode(err); err != nil {
            glog.Errorf("Error: ", err)
        }
    }
}

func MemoryUtilizationHandler(w http.ResponseWriter, r *http.Request) {
    u, err := url.Parse(fmt.Sprintf("http://%s:%d", DBHost, DBPort))
    if err != nil {
        glog.Fatalf("Error: ", err)
    }

    client, err := influxdb.NewClient(influxdb.Config {
        URL: *u,
        Username: DBUSer,
        Password: DBPasswd,
    })
    if err != nil {
        glog.Fatalf("Error: ", err)
    }

    vars := mux.Vars(r)
    host_name := vars["host-name"]

    query_cmd := fmt.Sprintf("SELECT * FROM /(%s.memory).*/", host_name)
    res, err := queryDB(client, query_cmd)
    if err == nil {
        json.NewEncoder(w).Encode(res[0].Series)
    } else {
        w.Header().Set("Content-Type", "application/json;charset=UTF-8")
        w.WriteHeader(422)
        if err := json.NewEncoder(w).Encode(err); err != nil {
            glog.Errorf("Error: ", err)
        }
    }
}
