#!py

import logging
import socket
import json
import random
import string

SKYRING_EVENT_SOCKET_FILE = "/tmp/echo.sock"
JSON_MAX_RECV_BYTES = 4096
log = logging.getLogger(__name__)


class JSONRPCError(Exception):
    pass


class PushEventException(Exception):
    pass


def _random_get_str():
    # due to the issue https://github.com/saltstack/salt/issues/25681
    # __salt__['random.get_str']() does not work
    # till then this function survives
    return ''.join(random.SystemRandom().choice(
        string.ascii_lowercase +
        string.ascii_uppercase +
        string.digits) for _ in range(20))


def call(sock, method, *args):
    request = dict(id=_random_get_str(),
                   params=args,
                   method=method)
    s = json.dumps(request).encode()
    sock.sendall(s)

    response = sock.recv(JSON_MAX_RECV_BYTES)
    try:
        response = json.loads(response.decode())
    except ValueError, e:
        raise JSONRPCError("JSONRPC: invalid JSON response. %s" % e)

    if response.get('id') != request.get('id'):
        raise JSONRPCError("JSONRPC: expected id=%s, received id=%s: %s"
                           % (request.get('id'),
                              response.get('id'),
                              response))

    error = response.get('error')
    if error:
        return None, error

    return response.get('result'), None


def PushEvent(data):
    sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    sock.connect(SKYRING_EVENT_SOCKET_FILE)
    result, error = call(sock, "Listener.PushEvent", json.dumps(data).encode())
    sock.close()
    if error:
        raise PushEventException(error)
    return result


def run():
    # tag and data variables are from salt
    try:
        result = PushEvent(data)
        if not result:
            log.error("PushEvent returns %s" % result)
    except (socket.error, JSONRPCError, PushEventException) as e:
        log.error(e, exc_info=True)

    return {}
