#!/usr/bin/env python
__doc__ = """
srndv2 api base
"""

import json
import os
import socket
import threading
import traceback
import logging
import queue

class SRNdAPI(threading.Thread):
    
    def __init__(self, addr, name):
        threading.Thread.__init__(self)
        self.log = logging.getLogger('SRNdAPI-{}'.format(name))
        self.addr = addr
        self.serv = socket.socket(socket.AF_UNIX)
        self.sock = socket.socket(socket.AF_UNIX)
        if os.path.exists(self.addr):
            os.unlink(self.addr)
        self.serv.bind(self.addr)
        self.serv.listen(5)
        self.sendlock = threading.Lock()
        
    def send(self, j):
        self.sendlock.acquire()
        if j is not None:
            d = json.dumps(j)
            print (d)
            self.sock.send(d+'\n.\n')
        self.sendlock.release()
            
    def please(self, name, **kwds):
        self.log.debug('please {}'.format(name))
        kwds['please'] = name
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
    
    def client(self, sock):
        f = sock.makefile()
        buff = ''
        while True:
            try:
                line = f.readline()
            except Exception as e:
                self.log.error(e)
                return
            else:
                if line == '':
                    break
                if line == '.\n':
                    try:
                        self.got(json.loads(buff))
                    except:
                        traceback.print_exc()
                    finally:
                        buff = ''
                else:
                    buff += line
                
    def run(self):
        while True:
            try:
                pair = self.serv.accept()
                if pair:
                    sock, addr = pair
                    self.client(sock)
                else:
                    return
            except:
                return
    
    def __del__(self):
        self.close()
        
