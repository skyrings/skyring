#!py

import socket
import json

SKYRING_EVENT_SOCKET_FILE = "/tmp/echo.sock"


class JSONRPCError(Exception):
    pass


class PushEventException(Exception):
    pass


def PushEvent(data):
    sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    sock.connect(SKYRING_EVENT_SOCKET_FILE)

    s = json.dumps(data).encode()
    request = dict(id='1234',
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
    except (socket.error, JSONRPCError, PushEventException) as e:
        result = str(e)

    return {'PushEvent': {'result': result}}
