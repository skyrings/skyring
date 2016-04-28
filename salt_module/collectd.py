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

import utils
import re
import ast

resource_collectd_table_dict = {
    'memory': ['memory/memory-used', 'aggregation-memory-sum/memory', 'memory/percent-used'],
    'cpu': ['cpu/percent-user'],
    'swap': ['swap/swap-used', 'aggregation-swap-sum/swap', 'swap/percent-used'],
}

collectd_table_stat_type = {
    'memory/memory-used': 'memory-used',
    'aggregation-memory-sum/memory': 'memory-total',
    'memory/percent-used': 'memory-percent-used',
    'cpu/percent-user': 'cpu-user-percent-used',
    'swap/swap-used': 'swap-used',
    'aggregation-swap-sum/swap': 'swap-total',
    'swap/percent-used': 'swap-percent-used',
}

def getMetricFromCollectd(table_name):
    cmd=["salt-call", "grains.get", "id", "--out=json"]
    rc, out, err = utils.execCmd(cmd)
    if not rc:
        table_name = ast.literal_eval(out)["local"] + "/" + table_name
        exp=re.compile('[a-z]+\=[0-9]+[.][0-9]+[e][\+|\-][0-9]+')
        cmd = ["collectdctl", "getval", table_name]
        rc, out, err = utils.execCmd(cmd)
        values = {}
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
        return err
    return values


def getMetricsFromCollectd(resource_name):
    ret_val = {}
    table_val = {}
    for table_name in resource_collectd_table_dict[resource_name]:
        table_val[collectd_table_stat_type[table_name]] = getMetricFromCollectd(table_name)
    return table_val
