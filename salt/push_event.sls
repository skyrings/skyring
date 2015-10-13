#!py

import logging
import socket
import json
import random
import string
import fnmatch
from datetime import datetime
import pytz

SKYRING_EVENT_SOCKET_FILE = "/tmp/.skyring-event"
JSON_MAX_RECV_BYTES = 4096
log = logging.getLogger(__name__)


class JSONRPCError(Exception):
    pass


def _random_get_str():
    # due to the issue https://github.com/saltstack/salt/issues/25681
    # __salt__['random.get_str']() does not work
    # till then this function survives
    return ''.join(random.SystemRandom().choice(
        string.ascii_lowercase +
        string.ascii_uppercase +
        string.digits) for _ in range(20))


class DateTimeEncoder(json.JSONEncoder):
    def default(self, o):
        if isinstance(o, datetime):
            return o.isoformat()

        return json.JSONEncoder.default(self, o)


class JsonRpcClient:
    def __init__(self, socketFile=SKYRING_EVENT_SOCKET_FILE,
                 maxReceive=JSON_MAX_RECV_BYTES):
        self.socketFile = socketFile
        self.maxReceive = maxReceive
        self.sock = None
        self.connected = False

    def open(self):
        if not self.connected:
            self.sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
            self.sock.connect(self.socketFile)
            self.connected = True

    def close(self):
        if self.connected:
            self.sock.close()

    def call(self, method, *args):
        if not self.connected:
            self.open()

        request = dict(id=_random_get_str(),
                       params=args,
                       method=method)
        self.sock.sendall(json.dumps(request, cls=DateTimeEncoder).encode())

        response = self.sock.recv(self.maxReceive)
        try:
            response = json.loads(response.decode())
        except ValueError, e:
            raise JSONRPCError("invalid JSON response. %s" % e)

        if response.get('id') != request.get('id'):
            raise JSONRPCError(
                "id mismatch.  request-id=%s response-id=%s" %
                (request.get('id'), response.get('id')))

        error = response.get('error')
        if error:
            raise JSONRPCError("error in method execution. %s: %s" %
                               (method, error))

        result = response.get('result')
        if not result:
            log.warn("method %s returns empty or false" % method)
        return result

    def __enter__(self):
        self.open()
        return self

    def __exit__(self, type, value, traceback):
        if type:
            try:
                self.close()
            except socket.error as e:
                log.error(e, exc_info=True)
        else:
            self.close()


def PushNodeStartEvent(data):
    naiveTime = datetime.strptime(data['_stamp'], "%Y-%m-%dT%H:%M:%S.%f")
    timestamp = naiveTime.replace(tzinfo=pytz.UTC)
    node = data['id']
    try:
        with JsonRpcClient() as c:
            c.call("Listener.PushNodeStartEvent",
                   {'timestamp': timestamp, 'node': node})
    except (socket.error, JSONRPCError) as e:
        log.error(e, exc_info=True)

def PushNodeDbusEvent(data):
    naiveTime = datetime.strptime(data['_stamp'], "%Y-%m-%dT%H:%M:%S.%f")
    timestamp = naiveTime.replace(tzinfo=pytz.UTC)
    node = data['id']
    tag = data['tag']
    message = data['data']['message']
    severity = data['data']['severity']
    try:
        with JsonRpcClient() as c:
            c.call("Listener.PersistNodeDbusEvent",
                   {'timestamp': timestamp, 'node': node, 'tag': tag, 'message': message,
                    'severity': severity})
    except (socket.error, JSONRPCError) as e:
        log.error(e, exc_info=True)


def PushEvent(data):
    try:
        with JsonRpcClient() as c:
            c.call("Listener.PushEvent", json.dumps(data).encode())
    except (socket.error, JSONRPCError) as e:
        log.error(e, exc_info=True)


def run():
    # tag and data variables are from salt
    if data.get('tag') and fnmatch.fnmatch(data['tag'], 'salt/minion/*/start'):
        PushNodeStartEvent(data)
    elif data.get('tag') and fnmatch.fnmatch(data['tag'], 'dbus/node/*'):
        PushNodeDbusEvent(data)
    else:
        PushEvent(data)
    return {}
