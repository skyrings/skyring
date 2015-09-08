import logging
import socket
from jinja2 import Template

import salt
from salt import wheel, client
import salt.config
import salt.utils.event
import salt.runner

import utils


SETUP_NODE_TEMPLATE = 'setup-node.sh.template'
log = logging.getLogger(__name__)
opts = salt.config.master_config('/etc/salt/master')
master = salt.wheel.WheelClient(opts)
sevent = salt.utils.event.get_event('master',
                                    sock_dir=opts['sock_dir'],
                                    transport=opts['transport'],
                                    opts=opts)
runner = salt.runner.RunnerClient(opts)
DEFAULT_WAIT_TIME = 5
setattr(salt.client.LocalClient, 'cmd',
        utils.enableLogger(salt.client.LocalClient.cmd))
local = salt.client.LocalClient()


def get_managed_nodes():
    keys = master.call_func('key.list_all')
    return {'accepted_nodes': keys['minions'],
            'denied_nodes': keys['minions_denied'],
            'unaccepted_nodes': keys['minions_pre'],
            'rejected_nodes': keys['minions_rejected']}


def get_node_ssh_fingerprint(node):
    ## TODO: this code should be in core
    return utils.get_host_ssh_key(node)[0]


def add_node(node, fingerprint, username, password,
             skyring_master=socket.getfqdn()):
    t = Template(open(SETUP_NODE_TEMPLATE).read())
    cmd = t.render(skyring_master=skyring_master)
    utils.rexecCmd(str(cmd), node, fingerprint=fingerprint,
                   username=username, password=password)

    accepted = master.call_func('key.accept', match=node)
    return (True if accepted else False)
