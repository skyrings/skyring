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
	"github.com/gorilla/mux"
	"github.com/skyrings/skyring/conf"
	"github.com/skyrings/skyring/db"
	"github.com/skyrings/skyring/utils"
	"net/http"
	"strings"

	influxdb "github.com/influxdb/influxdb/client"
)

func queryDB(cmd string) (res []influxdb.Result, err error) {
	q := influxdb.Query{
		Command:  cmd,
		Database: conf.SystemConfig.TimeSeriesDBConfig.Database,
	}

	if response, err := db.GetMonitoringDBClient().Query(q); err == nil {
		if response.Error() != nil {
			return res, response.Error()
		}
		res = response.Results
	}
	return
}

func GET_Utilization(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	node_id := vars["node-id"]

	params := r.URL.Query()
	resource_name := params.Get("resource")
	duration := params.Get("duration")

	storage_node := GetNode(node_id)

	var query_cmd string
	if resource_name != "" {
		query_cmd = fmt.Sprintf("SELECT * FROM /(%s.%s).*/", storage_node.Hostname, resource_name)
	} else {
		query_cmd = fmt.Sprintf("SELECT * FROM /(%s).*/", storage_node.Hostname)
	}

	if duration != "" {
		if strings.Contains(duration, ",") {
			splt := strings.Split(duration, ",")
			start_time := splt[0]
			end_time := splt[1]
			query_cmd += " WHERE time > '" + start_time + "' and time < '" + end_time + "'"
		} else {
			query_cmd += " WHERE time > now() - " + duration
		}
	}

	res, err := queryDB(query_cmd)
	if err == nil {
		json.NewEncoder(w).Encode(res[0].Series)
	} else {
		util.HttpResponse(w, http.StatusInternalServerError, err.Error())
	}
}
