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

import salt
from salt import wheel, client
import salt.config


log = logging.getLogger(__name__)


def enableLogger(func):
    @wraps(func)
    def wrapper(*args, **kwargs):
        print 'args=%s, kwargs=%s' % (args, kwargs)
        log.info('args=%s, kwargs=%s' % (args, kwargs))
        rv = func(*args, **kwargs)
        print 'rv=%s' % rv
        log.info('rv=%s' % rv)
        return rv
    return wrapper


opts = salt.config.master_config('/etc/salt/master')
master = salt.wheel.WheelClient(opts)
setattr(salt.client.LocalClient, 'cmd',
        enableLogger(salt.client.LocalClient.cmd))
local = salt.client.LocalClient()


def _get_keys(match='*'):
    keys = master.call_func('key.finger', match=match)
    return {'accepted_nodes': keys.get('minions', {}),
            'denied_nodes': keys.get('minions_denied', {}),
            'unaccepted_nodes': keys.get('minions_pre', {}),
            'rejected_nodes': keys.get('minions_rejected', {})}


def AcceptNode(node, fingerprint):
    d = _get_keys(node)
    if d['accepted_nodes'].get(node):
        log.info("node %s already in accepted node list" % node)
        return True

    finger = d['unaccepted_nodes'].get(node)
    if not finger:
        log.warn("node %s not in unaccepted node list" % node)
        return False

    if finger != fingerprint:
        log.error(("node %s minion fingerprint does not match %s != %s" %
                   (node, finger, fingerprint)))
        return False

    accepted = master.call_func('key.accept', match=node)
    return (True if accepted else False)


def GetNodes():
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


def GetNodeID(node):
    '''
    returns structure
    {"nodename": "uuidstring", ...}
    '''
    if type(node) is list:
        minions = node
    else:
        minions = [node]

    out = local.cmd(minions, 'grains.item', ['machine_id'], expr_form='list')
    rv = {}
    for minion in minions:
        rv[minion] = out.get(minion, {}).get('machine_id')
    return rv


def GetNodeNetwork(node):
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


def GetNodeDisk(node):
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
    out = local.cmd(minions, 'cmd.run', [lsblk], expr_form='list')

    minion_dev_info = {}
    for minion in minions:
        lsblk_out = out.get(minion)

        if not lsblk_out:
            minion_dev_info[minion] = {}
            continue

        devlist = map(lambda line: dict(zip(keys, line.split(' '))),
                      lsblk_out.splitlines())

        parents = set([d['PKNAME'] for d in devlist])

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
                              "Vendor": disk.get("VENDOR", "")})
    return rv
