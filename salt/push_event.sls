#!py

import logging
import socket
import json
import random
import string

SKYRING_EVENT_SOCKET_FILE = "/tmp/echo.sock"
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


def PushEvent(data):
    sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    sock.connect(SKYRING_EVENT_SOCKET_FILE)

    s = json.dumps(data).encode()
    request = dict(id=_random_get_str(),
                   params=[s],
                   method="Listener.PushEvent")
    s = json.dumps(request).encode()
    sock.sendall(s)

    response = sock.recv(4096)
    response = json.loads(response.decode())

    if response.get('id') != request.get('id'):
        raise JSONRPCError("expected id=%s, received id=%s: %s"
                           % (request.get('id'),
                              response.get('id'),
                              response))

    error = response.get('error')
    if error:
        raise PushEventException(error)

    sock.close()
    return response.get('result')


def run():
    # tag and data variables are from salt
    try:
        result = PushEvent(data)
        if not result:
            log.error("PushEvent returns %s" % result)
    except (socket.error, JSONRPCError, PushEventException) as e:
        log.error("PushEvent failed", exc_info=True)

    return {}
