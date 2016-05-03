#
# Copyright 2016 Red Hat, Inc.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

import skyring_utils
import re
import ast

resource_collectd_table_dict = {
    'memory': ['memory/memory-used', 'aggregation-memory-sum/memory', 'memory/percent-used'],
    'cpu': ['cpu/percent-user'],
    'swap': ['swap/swap-used', 'aggregation-swap-sum/swap', 'swap/percent-used'],
    'network': ['interface-average/bytes-total_bandwidth', 'interface-average/bytes-total_bandwidth_used', 'interface-average/percent-network_utilization'],
}

collectd_table_stat_type = {
    'memory/memory-used': 'Used',
    'aggregation-memory-sum/memory': 'Total',
    'memory/percent-used': 'PercentUsed',
    'cpu/percent-user': 'PercentUsed',
    'swap/swap-used': 'Used',
    'aggregation-swap-sum/swap': 'Total',
    'swap/percent-used': 'PercentUsed',
    'interface-average/bytes-total_bandwidth': 'Total',
    'interface-average/bytes-total_bandwidth_used': 'Used',
    'interface-average/percent-network_utilization': 'PercentUsed',
}

def getMetricFromCollectd(table_name):
    cmd=["salt-call", "grains.get", "id", "--out=json"]
    values = {}
    try:
        rc, out, err = skyring_utils.execCmd(cmd)
        if not rc:
            table_name = ast.literal_eval(out)["local"] + "/" + table_name
            exp=re.compile('[a-z]+\=[0-9]+[.][0-9]+[e][\+|\-][0-9]+')
            cmd = ["collectdctl", "getval", table_name]
            rc, out, err = skyring_utils.execCmd(cmd)
            if not rc:
                for index, val in enumerate(out.split()):
                    if exp.match(val):
                        keyValue = val.split("=")
                        if len(keyValue) == 2:
                            values[keyValue[0]] = keyValue[1]
                        else:
                            values["error"] = val
                    else:
                        values["error"] = out
            else:
                values["error"] = err
        else:
            values["error"] = err
        return values
    except skyring_utils.CmdExecFailed as e:
        values["error"] = str(e)
        return values


def getSingleValuedMetricsFromCollectd(resource_name):
    ret_val = {}
    table_val = {}
    for table_name in resource_collectd_table_dict[resource_name]:
        if "value" in getMetricFromCollectd(table_name):
            table_val[collectd_table_stat_type[table_name]] = getMetricFromCollectd(table_name)["value"]
        else:
            table_val[collectd_table_stat_type[table_name]] = getMetricFromCollectd(table_name)["error"]
    return table_val


def getMetricsFromCollectd(resource_name):
    ret_val = {}
    table_val = {}
    for table_name in resource_collectd_table_dict[resource_name]:
        table_val[collectd_table_stat_type[table_name]] = getMetricFromCollectd(table_name)
    return table_val
