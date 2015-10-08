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


def accept_node(node, fingerprint):
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


def get_nodes():
    return _get_keys()


def get_node_machine_id(node):
    if type(node) is list:
        minions = node
    else:
        minions = [node]

    out = local.cmd(minions, 'grains.item', ['machine_id'], expr_form='list')
    rv = {}
    for minion in minions:
        rv[minion] = out.get(minion, {}).get('machine_id')
    return rv


def get_node_network_info(node):
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
            netinfo[minion] = {'ipv4': info['grains.item']['ipv4'],
                               'ipv6': info['grains.item']['ipv6'],
                               'subnet': info['network.subnets']}
        else:
            netinfo[minion] = {'ipv4': [], 'ipv6': [], 'subnet': []}

    return netinfo

def get_volume_utilization(node, volumeName, warningLimit, criticalLimit):
    return local.cmd(node, 'cmd.run', ["/usr/lib64/nagios/plugins/gluster/check_vol_utilization.py" +  volumeName + " -w " + warningLimit + " -c " + criticalLimit])

def get_node_disk_info(node):
    '''
    This function returns disk/storage device info excluding their
    parent devices

    Output dictionary is
    {DEV_MAME: {'INUSE': BOOLEAN,
                'NAME': SHORT_NAME,
                'KNAME': DEV_NAME,
                'FSTYPE': FS_TYPE,
                'MOUNTPOINT': MOUNT_POINT,
                'UUID': FS_UUID,
                'PARTUUID': PART_UUID,
                'MODEL': MODEL_STRING,
                'SIZE': SIZE_BYTES,
                'TYPE': TYPE,
                'PKNAME', PARENT_DEV_NAME,
                'VENDOR': VENDOR_STRING}, ...}
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

    return minion_dev_info
