from . import network
import asyncio
import logging
import nntplib
import threading
import unittest

import pytest


class DaemonTest(unittest.TestCase):

    def setUp(self):
        self.groups = 'overchan.test,ctl'
        conf = dict()
        conf['bind_host'] = '::1'
        conf['bind_port'] = '11199'
        conf['groups'] = self.groups
        self.daemon = network.NNTPD(conf)
        self.daemon.start()
        loop = asyncio.get_event_loop()

    def nntp(self):
        return nntplib.NNTP('::1', 11199, timeout=0.5)

        
    def nntp_check_caps(self, ftr):
        self.cl = self.nntp()
        caps = self.cl.getcapabilities()
        assert 'POST' in caps
        assert 'STREAM' in caps
        ftr.set_result(True)

    def nntp_check_groups(self, ftr):
        print ('check groups')
        self.cl = self.nntp()
        for group in self.groups.split(','):
            print ('check {}'.format(group))
            self.cl.groups(group)
        ftr.set_result(True)

    def end(self):
        self.daemon.end()
        

    def test_all(self):
        loop = asyncio.get_event_loop()
        try:
            for func in (self.nntp_check_caps,):
                ftr = asyncio.Future()
                def runit():
                    try:
                        func(ftr)
                    except Exception as e:
                        ftr.set_result(False)
                        self.end()
                threading.Thread(target=runit).start()
                res = loop.run_until_complete(asyncio.wait_for(ftr, 1))
                if res is False:
                    pytest.fail('test failed for {}'.format(func.__name__))
                
        except asyncio.TimeoutError:
            pytest.fail('timeout')
            
