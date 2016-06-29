#
# Copyright 2015 Red Hat, Inc.
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

import logging
from functools import wraps
import uuid
from sets import Set
import salt
from salt import wheel, client, key, states, modules
import salt.config

log = logging.getLogger(__name__)


def enableLogger(func):
    @wraps(func)
    def wrapper(*args, **kwargs):
        log.info('args=%s, kwargs=%s' % (args, kwargs))
        rv = func(*args, **kwargs)
        log.info('rv=%s' % rv)
        return rv
    return wrapper


opts = salt.config.master_config('/etc/salt/master')
setattr(salt.client.LocalClient, 'cmd',
        enableLogger(salt.client.LocalClient.cmd))


def _get_keys(match='*'):
    master = salt.wheel.WheelClient(opts)
    keys = master.call_func('key.finger', match=match)
    return {'accepted_nodes': keys.get('minions', {}),
            'denied_nodes': keys.get('minions_denied', {}),
            'unaccepted_nodes': keys.get('minions_pre', {}),
            'rejected_nodes': keys.get('minions_rejected', {})}


def AcceptNode(node, fingerprint, include_rejected=False, ctxt=""):
    d = _get_keys(node)
    if d['accepted_nodes'].get(node):
        log.info("%s-node %s already in accepted node list" % (ctxt, node))
        return True

    finger = d['unaccepted_nodes'].get(node)
    if not finger:
        if include_rejected:
            finger = d['rejected_nodes'].get(node)
    if not finger:
        log.warn("%s-node %s not in unaccepted/rejected node list" % (ctxt, node))
        return False

    if finger != fingerprint:
        log.error(("%s-node %s minion fingerprint does not match %s != %s" %
                   (ctxt, node, finger, fingerprint)))
        return False

    skey = salt.key.Key(opts)
    out = skey.accept(match=node, include_rejected=include_rejected)

    return (node in out['minions'])


def IgnoreNode(node, ctxt=""):
    skey = salt.key.Key(opts)
    out = skey.delete_key(node)
    return True


def GetNodes(ctxt=""):
    '''
    returns structure
    {"Manage":   [{"Name": "nodename", "Fingerprint": "nodefinger"}, ...],
     "Unmanage": [{"Name": "nodename", "Fingerprint": "nodefinger"}, ...],
     "Ignore":   [{"Name": "nodename", "Fingerprint": "nodefinger"}, ...]}
    '''
    keys = _get_keys()

    manage = []
    for name, finger in keys.get("accepted_nodes", {}).iteritems():
        manage.append({"Name": name, "Fingerprint": finger})

    unmanage = []
    ignore = []
    for name, finger in keys.get("unaccepted_nodes", {}).iteritems():
        unmanage.append({"Name": name, "Fingerprint": finger})

    ignore = []
    for name, finger in keys.get("rejected_nodes", {}).iteritems():
        ignore.append({"Name": name, "Fingerprint": finger})
    for name, finger in keys.get("denied_nodes", {}).iteritems():
        ignore.append({"Name": name, "Fingerprint": finger})

    return {"Manage": manage, "Unmanage": unmanage, "Ignore": ignore}


def GetNodeID(node, ctxt=""):
    '''
    returns structure
    {"nodename": "uuidstring", ...}
    '''
    if type(node) is list:
        minions = node
    else:
        minions = [node]

    local = salt.client.LocalClient()
    out = local.cmd(minions, 'grains.item', ['machine_id'], expr_form='list')
    rv = {}
    for minion in minions:
        rv[minion] = out.get(minion, {}).get('machine_id')
    return rv


def GetNodeNetwork(node, ctxt=""):
    '''
    returns structure
    {"nodename": {"IPv4": ["ipv4address", ...],
                  "IPv6": ["ipv6address", ...],
                  "Subnet": ["subnet", ...]}, ...}
    '''
    if type(node) is list:
        minions = node
    else:
        minions = [node]

    local = salt.client.LocalClient()
    out = local.cmd(minions, ['grains.item', 'network.subnets'],
                    [['ipv4', 'ipv6'], []], expr_form='list')
    netinfo = {}
    for minion in minions:
        info = out.get(minion)
        if info:
            netinfo[minion] = {'IPv4': info['grains.item']['ipv4'],
                               'IPv6': info['grains.item']['ipv6'],
                               'Subnet': info['network.subnets']}
        else:
            netinfo[minion] = {'IPv4': [], 'IPv6': [], 'Subnet': []}

    return netinfo


def GetNodeDisk(node, ctxt=""):
    '''
    returns structure
    {"nodename": [{"DevName":   "devicename",
                  "FSType":     "fstype",
                  "FSUUID":     "uuid",
                  "Model":      "model",
                  "MountPoint": ["mountpoint", ...],
                  "Name":       "name",
                  "Parent":     "parentdevicename",
                  "Size":       uint64,
                  "Type":       "type",
                  "Used":       boolean,
                  "SSD":        boolean,
                  "Vendor":     "string"}, ...], ...}
    '''

    if type(node) is list:
        minions = node
    else:
        minions = [node]

    columes = 'NAME,KNAME,FSTYPE,MOUNTPOINT,UUID,PARTUUID,MODEL,SIZE,TYPE,' \
              'PKNAME,VENDOR'
    keys = columes.split(',')
    lsblk = ("lsblk --all --bytes --noheadings --output='%s' --path --raw" %
             columes)
    local = salt.client.LocalClient()
    out = local.cmd(minions, 'cmd.run', [lsblk], expr_form='list')

    minion_dev_info = {}
    for minion in minions:
        lsblk_out = out.get(minion)

        if not lsblk_out:
            minion_dev_info[minion] = {}
            continue

        devlist = map(lambda line: dict(zip(keys, line.split(' '))),
                      lsblk_out.splitlines())

        parents = set([d['PKNAME'] for d in devlist if d.has_key('PKNAME')])

        dev_info = {}
        for d in devlist:
            in_use = True

            if d['TYPE'] == 'disk':
                if d['KNAME'] in parents:
                    # skip it
                    continue
                else:
                    in_use = False
            elif not d['FSTYPE']:
                in_use = False

            d.update({'INUSE': in_use})
            dev_info.update({d['KNAME']: d})

        minion_dev_info[minion] = dev_info

    rv = {}
    for node, ddict in minion_dev_info.iteritems():
        rv[node] = []
        for disk in ddict.values():
            try:
                u = list(bytearray(uuid.UUID(disk["UUID"]).get_bytes()))
            except ValueError:
                # TODO: log the error
                u = [0] * 16
            if disk['TYPE'] == 'disk':
                ssdStat = isSSD(node, disk['KNAME'])
            else:
                ssdStat = False
            if disk["SIZE"] == "":
                log.error(
                    "%s-Skipping the disk: %s as size field is blank." %
                    (ctxt, disk["NAME"]))
                continue
            rv[node].append({"DevName": disk["KNAME"],
                              "FSType": disk["FSTYPE"],
                              "FSUUID": u,
                              "Model": disk["MODEL"],
                              "MountPoint": [disk["MOUNTPOINT"]],
                              "Name": disk["NAME"],
                              "Parent": disk["PKNAME"],
                              "Size": long(disk["SIZE"]),
                              "Type": disk["TYPE"],
                              "Used": disk["INUSE"],
                              "SSD": ssdStat,
                              "Vendor": disk.get("VENDOR", ""),
                              "StorageProfile": "",
                              "DiskId":u})
    return rv

def GetNodeCpu(node, ctxt=""):
    '''
    returns structure
    {"nodename": [{"Architecture":   "architecture",
                   "CpuOpMode":      "cpuopmode",
                   "CPUs":           "cpus",
                   "VendorId":       "vendorid",
                   "ModelName":      "modelname",
                   "CPUFamily":      "cpufamily",
                   "CPUMHz":         "cpumhz",
                   "Model":          "Model",
                   "CoresPerSocket": "corespersocket"}, ...], ...}
    '''
    if type(node) is list:
        minions = node
    else:
        minions = [node]

    lscpu = ("lscpu")
    local = salt.client.LocalClient()
    out = local.cmd(minions, 'cmd.run', [lscpu], expr_form='list')
    cpuinfo = {}
    for minion in minions:
        info = out.get(minion)
        if info:
            info_list = info.split('\n')
            cpuinfo[minion] = [{'Architecture': info_list[0].split(':')[1].strip(),
                               'CpuOpMode': info_list[1].split(':')[1].strip(),
                               'CPUs': info_list[3].split(':')[1].strip(),
                               'VendorId': info_list[9].split(':')[1].strip(),
                               'ModelName': info_list[12].split(':')[1].strip(),
                               'CPUFamily': info_list[10].split(':')[1].strip(),
                               'CPUMHz': info_list[14].split(':')[1].strip(),
                               'Model': info_list[11].split(':')[1].strip(),
                               'CoresPerSocket': info_list[6].split(':')[1].strip()}]
        else:
            cpuinfo[minion] = [{'Architecture': '', 'CpuOpMode': '', 'CPUs': '', 'VendorId': '', 'ModelName': '', 'CPUFamily': '', 'CPUMHz': '', 'Model': '', 'CoresPerSocket': ''}]

    return cpuinfo


def GetNodeOs(node, ctxt=""):
    if type(node) is list:
        minions = node
    else:
        minions = [node]
    local = salt.client.LocalClient()
    os_out = local.cmd(minions, 'grains.item', ['osfullname'], expr_form='list')
    os_version_out = local.cmd(minions, 'grains.item', ['osrelease'], expr_form='list')
    uname = ("uname --all")
    uname_out = local.cmd(minions, 'cmd.run', [uname], expr_form='list')
    se = ("getenforce")
    se_out = local.cmd(minions, 'cmd.run', [se], expr_form='list')
    osinfo = {}
    for minion in minions:
        os = os_out.get(minion).get("osfullname")
        os_version = os_version_out.get(minion).get("osrelease")
        uname_info = uname_out.get(minion)
        if os and os_version and uname_info:
            uname_info_list = uname_info.split(' ')
            osinfo[minion] = {'Name': os,
                              'OSVersion': os_version,
                              'KernelVersion': uname_info_list[2],
                              'SELinuxMode': se_out.get(minion)}
        else:
            osinfo[minion] = {'Name': '', 'OSVersion': '', 'KernelVersion': '', 'SELinuxMode': ''}
    return osinfo


def GetNodeMemory(node, ctxt=""):
    '''
    returns structure
    {"nodename": [{"TotalSize": "totalsize",
                   "SwapTotal": "swaptotal",
                   "Active":    "active",
                   "Type":      "type"}, ...], ...}
    '''

    if type(node) is list:
        minions = node
    else:
        minions = [node]

    vmstat = ("cat /proc/meminfo")
    local = salt.client.LocalClient()
    out = local.cmd(minions, 'cmd.run', [vmstat], expr_form='list')

    memoinfo = {}
    for minion in minions:
        info = out.get(minion)
        if info:
            info_list = info.split('\n')
            memoinfo[minion] = {'TotalSize': info_list[0].split(':')[1].strip(),
                                'SwapTotal': info_list[14].split(':')[1].strip(),
                                'Active': info_list[6].split(':')[1].strip(),
                                'Type': ''}
        else:
            memoinfo[minion] = {'TotalSize': '', 'SwapTotal': '', 'Active': '', 'Type': ''}

    return memoinfo

def DisableService(node, service, stop=False, ctxt=""):
    local = salt.client.LocalClient()
    out = local.cmd(node, 'service.disable', [service])
    if out[node] and stop:
        out = local.cmd(node, 'service.stop', [service])

    return out[node]


def EnableService(node, service, start=False, ctxt=""):
    local = salt.client.LocalClient()
    out = local.cmd(node, 'service.enable', [service])
    if out[node] and start:
        out = local.cmd(node, 'service.start', [service])

    return out[node]


def SyncModules(node, ctxt=""):
    local = salt.client.LocalClient()
    out = local.cmd(node, 'saltutil.sync_all')
    return out[node]


def NodeUp(node, ctxt=""):
    local = salt.client.LocalClient()
    out = local.cmd(node, 'test.ping')
    return True if out.has_key(node) and out[node] else False


def NodeUptime(node, ctxt=""):
    local = salt.client.LocalClient()
    out = local.cmd(node,'cmd.run',['uptime -p'])
    return out[node]


def _get_state_result(out):
    failed_minions = {}
    for minion, v in out.iteritems():
        failed_results = {}
        for id, res in v.iteritems():
            if not res['result']:
                failed_results.update({id: res})
        if not v:
            failed_minions[minion] = {}
        if failed_results:
            failed_minions[minion] = failed_results
    return failed_minions


def run_state(tgts, state, *args, **kwargs):
    local = salt.client.LocalClient()
    out = local.cmd(
        tgts,
        'state.sls',
        [state],
        expr_form='list',
        *args,
        **kwargs)
    return _get_state_result(out)


# Monitoring Section
monitoring_root_path = '/etc/collectd.d/'
monitoring_disabled_root_path = '/etc/disabled_collectd.d/'


threshold_type_map = {"warning": "WarningMax", "critical": "FailureMax"}


def AddMonitoringPlugin(plugin_list, nodes, master=None, configs=None, ctxt=""):
    retVal = {}
    state_list = ''
    no_of_plugins = len(plugin_list)
    for index in range(no_of_plugins):
        state_list += ('collectd.' + plugin_list[index])
        if index < no_of_plugins - 1:
            state_list += ","
    plugin_thresholds = {}
    for plugin, value in configs.iteritems():
        thresholds = {}
        for threshold_type, threshold in value.iteritems():
            if not threshold_type_map.get(threshold_type):
                continue
            thresholds[threshold_type_map.get(threshold_type)] = threshold
        plugin_thresholds[plugin] = thresholds
    dict = {}
    if master:
        dict["master_name"] = master
    if thresholds:
        dict["thresholds"] = plugin_thresholds
    pillar = {"collectd": dict}
    state_load_result = run_state(nodes, state_list, kwarg={'pillar': pillar})
    if not state_load_result:
        for plugin_name, config in configs.iteritems():
            if config["Enable"] == "false":
                nodesInFailure = DisableMonitoringPlugin(nodes, plugin_name)
		if nodesInFailure:
                    retVal[plugin_name] = nodesInFailure
        return retVal
    else:
        return state_load_result


def DisableMonitoringPlugin(nodes, pluginName, ctxt=""):
    failed_minions = {}
    source_file = monitoring_root_path + pluginName + "*"
    destination = monitoring_disabled_root_path
    local = salt.client.LocalClient()
    local.cmd(nodes, "cmd.run", ["mkdir -p " + destination], expr_form='list')
    out = local.cmd(
        nodes, "cmd.run", [
            "mv " + source_file + " " + destination], expr_form='list')
    for nodeName, message in out.iteritems():
        if out.get(nodeName) != '':
            log.error(
                '%s-Failed to disable collectd plugin %s because %s on %s' %
                (ctxt, pluginName, out.get(nodeName), nodeName))
            failed_minions[nodeName] = out.get(nodeName)
    if failed_minions:
        return failed_minions
    out = local.cmd(nodes, 'service.restart', ['collectd'], expr_form='list')
    for node, restartStatus in out.iteritems():
        if not restartStatus:
            log.error("%s-Failed to restart collectd on node %s after disabling the plugin %s" %(ctxt, node, pluginName))
            failed_minions[node] = "Failed to restart collectd"
    return failed_minions

def SetupSkynetService(node, ctxt=""):
    if type(node) is list:
        minions = node
    else:
        minions = [node]
    status = True

    # Restart systemd-skynet service
    local = salt.client.LocalClient()
    out = local.cmd(minions, 'service.restart', ['skynetd'], expr_form='list')
    for node, restartStatus in out.iteritems():
        if not restartStatus:
            log.error("%s-Failed to restart skynetd on node %s " %(ctxt, node))
            status = False

    # Send a signal to storaged to enable modules. After this storaged will start sending signals 
    out = local.cmd(
        minions, "cmd.run", [
            "dbus-send --system --type=method_call --dest=org.storaged.Storaged /org/storaged/Storaged/Manager org.storaged.Storaged.Manager.EnableModules boolean:true"], expr_form='list')
    for nodeName, message in out.iteritems():
        if out.get(nodeName) != '':
            log.error(
                '%s-Failed to send signal to storaged on node: %s' % (ctxt, nodeName)
            )
            status = False
    return status

def EnableMonitoringPlugin(nodes, pluginName, ctxt=""):
    failed_minions = {}
    source_file = monitoring_disabled_root_path + pluginName + '.conf'
    destination = monitoring_root_path
    local = salt.client.LocalClient()
    out = local.cmd(
        nodes, "cmd.run", [
            "mv " + source_file + " " + destination], expr_form='list')
    for nodeName, message in out.iteritems():
        if out.get(nodeName) != '':
            log.error(
                '%s-Failed to enable collectd plugin %s because %s on %s' %
                (ctxt, pluginName, out.get(nodeName), nodeName))
            failed_minions[nodeName] = out.get(nodeName)
    if failed_minions:
        return failed_minions
    out = local.cmd(nodes, 'service.restart', ['collectd'], expr_form='list')
    for node, restartStatus in out.iteritems():
        if not restartStatus:
            log.error("%s-Failed to restart collectd on node %s after enabling the plugin %s" %(ctxt, node, pluginName))
            failed_minions[node] = "Failed to restart collectd"
    return failed_minions


def isSSD(node, device, ctxt=""):
    cmd = 'cat /sys/block/%s/queue/rotational' % device.split("/")[-1]
    local = salt.client.LocalClient()
    out = local.cmd('%s' % node, 'cmd.run', [cmd], expr_form='list').get(node, '').strip()
    if not out:
        log.error("%s-Failed to get cluster statistics from %s" % (ctxt, node))
        raise Exception("Failed to get cluster statistics from %s" % node)
    if out == '0':
        return True
    if out == '1':
        return False
    # Rotational attribute not found for this device which is not either SSD or HD
    return False


def RemoveMonitoringPlugin(nodes, pluginName, ctxt=""):
    failed_minions = {}
    source_file = monitoring_root_path + pluginName + "*"
    local = salt.client.LocalClient()
    out = local.cmd(
        nodes, "cmd.run", [
            "rm -fr " + source_file], expr_form='list')
    for nodeName, message in out.iteritems():
        if out.get(nodeName) != '':
            log.error(
                '%s-Failed to remove collectd plugin %s because %s on %s' %
                (ctxt, pluginName, out.get(nodeName), nodeName))
            failed_minions[nodeName] = out.get(nodeName)
    if failed_minions :
        return failed_minions
    out = local.cmd(nodes, 'service.restart', ['collectd'], expr_form='list')
    for node, restartStatus in out.iteritems():
        if not restartStatus:
            log.error("%s-Failed to restart collectd on node %s after removing the plugin %s" % (ctxt, node, pluginName))
            failed_minions[node] = "Failed to restart collectd"
    return failed_minions


def UpdateMonitoringConfiguration(nodes, plugin_threshold_dict, ctxt=""):
    failed_minions = {}
    local = salt.client.LocalClient()
    for key, value in plugin_threshold_dict.iteritems():
        path = monitoring_root_path + key + '.conf'
        for threshold_type, threshold in value.iteritems():
            pattern_type = threshold_type_map.get(threshold_type)
            if not pattern_type:
                continue
            pattern = pattern_type + " " + "[0-9]*"
            repl = pattern_type + " " + str(threshold)
            out = local.cmd(nodes, "file.replace", expr_form='list', kwarg={'path': path, 'pattern': pattern, 'repl': repl})
    out = local.cmd(nodes, 'service.restart', ['collectd'], expr_form='list')
    for node, restartStatus in out.iteritems():
        if not restartStatus:
            log.error("%s-Failed to restart collectd on node %s" % (ctxt, node))
            failed_minions[node] = "Failed to restart collectd"
    return failed_minions


def GetSingleValuedMetricFromCollectd(nodes, resource_name, ctxt=""):
    saltClient = salt.client.LocalClient()
    return saltClient.cmd(nodes, "collectd.getSingleValuedMetricsFromCollectd", [resource_name], expr_form='list')


def GetFingerPrint(node, ctxt=""):
    d = _get_keys(node)
    if d['unaccepted_nodes'].get(node):
        return d['unaccepted_nodes']
    log.error("%s-Failed to retrive fingerprint from host %s" % (ctxt, node))
    return False
