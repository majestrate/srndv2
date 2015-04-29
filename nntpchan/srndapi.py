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

class SRNdAPI:
    
    def __init__(self, loop, addr, name):
        self.log = logging.getLogger('SRNdAPI-{}'.format(name))
        self.name = name
        self.addr = addr
        self.serv = socket.socket(socket.AF_UNIX)
        self.sock = socket.socket(socket.AF_UNIX)
        self.loop = loop
        
    def bind(self):
        if os.path.exists(self.addr):
            os.unlink(self.addr)
        self.serv.bind(self.addr)
        self.serv.listen(5)
        
    def send(self, j):
        if j is not None:
            d = json.dumps(j)
            self.sock.send(d+'\n.\n')
            
    def please(self, cmd, **kwds):
        self.log.debug('please {} --> {}'.format(cmd, kwds))
        kwds['Please'] = cmd
        self.send(kwds)
        
    def connect(self):
        self.log.info('connecting to daemon')
        self.sock.connect("srnd.sock")
        self.please('socket', socket=self.addr)
        
    def close(self):
        self.log.info('closing')
        self.sock.close()
        self.serv.close()
    
    def got(self, obj):
        pass


    def _handle_serv(self, sock, fd, events):
        """
        handle socket events from srnd
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

        ## add event listener
        cb = funcutils.partial(self._handle_srnd_sock, conn)
        self.loop.add_handler(conn.fileno(), cb, io_loop.READ)

    def _handle_srnd_sock(self, sock, fd, events):
        """
        handle a connection from srnd
        """

        
    def _handle_sock(self, sock, fd, events):
        """
        handle socket events to srnd
        """
    
    def run(self):
        """
        run the daemon
        """
        self.bind()
        self._setup_sock(self.serv, self._handle_serv)
        self._setup_sock(self.sock, self._handle_sock)
        
    def _setup_sock(self, sock, handler):
        sock.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        sock.setblocking(0)
        cb = functools.partial(handler, sock)
        self.loop.add_handler(sock.fileno(), cb, io_loop.READ)
        
    def __del__(self):
        self.close()
        
