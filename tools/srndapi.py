#!/usr/bin/env python
__doc__ = """
srndv2 api tool
"""

import json
import os
import socket
import threading


class SRNdAPI(threading.Thread):
    
    def __init__(self, addr):
        threading.Thread.__init__(self)
        self.addr = addr
        self.serv = socket.socket(socket.AF_UNIX)
        self.sock = socket.socket(socket.AF_UNIX)
        if os.path.exists(self.addr):
            os.unlink(self.addr)
        self.serv.bind(self.addr)
        self.serv.listen(5)
        
    def send(self, j):
        if j is not None:
            d = json.dumps(j)
            print (d)
            self.sock.send(d+'\n.\n')
            
    def please(self, name, **kwds):
        print ('please {}'.format(name))
        kwds['please'] = name
        self.send(kwds)
        
    def connect(self):
        print ('connecting to daemon')
        self.sock.connect("srnd.sock")
        self.please('socket', socket=self.addr)
        
    def close(self):
        print ('closing')
        self.sock.close()
        self.serv.close()
    
    def got(self, obj):
        print('we got {}'.format(obj))
    
    def client(self, sock):
        f = sock.makefile()
        buff = ''
        while True:
            line = f.readline()
            if line == '':
                break
            if line == '.\n':
                self.got(json.loads(buff))
            else:
                buff += line
                
    def run(self):
        while True:
            pair = self.serv.accept()
            if pair:
                sock, addr = pair
                self.client(sock)
            else:
                return
    
    def __del__(self):
        self.close()
        
if __name__ == '__main__':
    api = SRNdAPI('test-frontend.sock')
    api.start()
    api.connect()