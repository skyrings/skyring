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

func queryDB(cmd string) (res []influxdb.Result, err error) {
    u, err := url.Parse(fmt.Sprintf("http://%s:%d", "influxdb", 8086))
    if err != nil {
        glog.Fatalf("Error: ", err)
    }

    client, err := influxdb.NewClient(influxdb.Config {
        URL: *u,
        Username: "admin",
        Password: "admin",
    })
    if err != nil {
        glog.Fatalf("Error: ", err)
    }

    q := influxdb.Query {
        Command: cmd,
        Database: "collectd",
    }

    if response, err := client.Query(q); err == nil {
        if response.Error() != nil {
            return res, response.Error()
        }
        res = response.Results
    }
    return
}

func ResourceUtilizationHandler(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    node_id := vars["node-id"]

    params := r.URL.Query()
    resource_name := params.Get("resource")

    storage_node := GetNode(node_id)

    var query_cmd string
    if resource_name != "" {
        query_cmd = fmt.Sprintf("SELECT * FROM /(%s.%s).*/", storage_node.Hostname, resource_name)
    } else {
        query_cmd = fmt.Sprintf("SELECT * FROM /(%s).*/", storage_node.Hostname)
    }
    res, err := queryDB(query_cmd)
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
