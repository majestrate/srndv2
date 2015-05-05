#!/usr/bin/env python
__doc__ = """
srndv2 api base
"""

import json
import os
import socket
import traceback
import logging
import queue
import functools
import errno

from tornado import iostream

class SRNdAPI:

    
    log = logging.getLogger('SRNdAPI')
    
    def __init__(self, loop, addr, name):
        self._name = name
        self._addr = addr
        self._serv = socket.socket(socket.AF_UNIX)
        self._sock = socket.socket(socket.AF_UNIX)
        self.loop = loop
        self._sendq = queue.Queue()
        self._stream = iostream.IOStream(self._sock)
        self._serv_stream = None
        self._setup_sock(self._serv, self._handle_serv)
                
    def bind(self):
        """
        bind socket for listening for events from srnd
        """
        if os.path.exists(self._addr):
            os.unlink(self._addr)
        self._serv.bind(self._addr)
        self._serv.listen(5)
        
    def send(self, j):
        """
        send a single message to srnd
        """
        if j is not None:
            d = json.dumps(j)
            d += "\n.\n"
            self._stream.write(d)
            
    def please(self, cmd, **kwds):
        """
        send a please command
        """
        self.log.debug('please {} --> {}'.format(cmd, kwds))
        kwds['Please'] = cmd
        self.send(kwds)

    def _connected_to_srnd(self):
        """
        called after connected to SRNd
        """
        self.log.info('connected!!!')
        # send please socket command
        self.please('socket', socket=self._addr)
        
    def connect(self, sock="srnd.sock"):
        """
        connect to srnd
        """
        self.log.info('connecting to daemon at {}'.format(sock))
        self._stream.connect(sock, self._connected_to_srnd)

        
    def close(self):
        """
        close all connections
        """
        self.log.info('closing')
        self._sock.close()
        self._serv.close()
        if os.path.exists(self._addr):
            os.unlink(self._addr)
        
    def got(self, obj):
        """
        called when we got a message from srnd
        """
        pass

    def _got_raw_from_srnd(self, data):
        """
        we got a raw message from srnd
        """
        try:
            j = json.loads(data[:-3])
        except Exception as e:
            self.log.error(e)
        else:
            self.log.debug("got {}".format(j))
            # process message
            self.got(j)
            # read next message
            self._read_next_from_srnd()
        
    def _handle_serv(self, sock, fd, events):
        """
        handle incoming connection from srnd
        """
        ## handle accept() gymnastics
        try:
            conn, addr = sock.accept()
        except socket.error as e:
            if e.args[0] not in (errno.EWOULDBLOCK, errno.EAGAIN):
                raise
            return
        ## set non blocking
        conn.setblocking(0)
        self._serv_stream = iostream.IOStream(conn)
        self._read_next_from_srnd()
        
    def _read_next_from_srnd(self):
        """
        read the next message from srnd
        """
        self._serv_stream.read_until("\n.\n", self._got_raw_from_srnd)
        
    def _handle_srnd_sock(self, sock, fd, events):
        """
        handle a connection from srnd
        """
    
    def _setup_sock(self, sock, handler):
        sock.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        sock.setblocking(0)
        cb = functools.partial(handler, sock)
        self.loop.add_handler(sock.fileno(), cb, self.loop.READ)

        
    def __del__(self):
        self.close()
        
