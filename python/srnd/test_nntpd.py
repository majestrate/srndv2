from . import network
import asyncio
import logging
import nntplib
import threading
import unittest

import pytest

logging.basicConfig(level=logging.DEBUG)

class DaemonTest(unittest.TestCase):

    def setUp(self):
        self.groups = 'overchan.test,ctl'
        daemon_conf = dict()
        daemon_conf['bind_host'] = '::1'
        daemon_conf['bind_port'] = '11199'
        daemon_conf['groups'] = self.groups
        daemon_conf['instance_name'] = 'test.tld'

        feed_conf = dict()
        feed_conf['default'] = dict()

        store_conf = dict()
        store_conf['base_dir'] = 'articles'

        self.daemon = network.NNTPD(daemon_conf, feed_conf, store_conf)
        self.daemon.start()
        loop = asyncio.get_event_loop()

    def nntp(self):
        return nntplib.NNTP('::1', 11199, timeout=0.5)

        
    def nntp_check_caps(self, ftr):
        self.cl = self.nntp()
        caps = self.cl.getcapabilities()
        assert 'POST' in caps
        assert 'STREAMING' in caps
        assert 'SOCIALISM' not in caps
        ftr.set_result(True)

    def nntp_check_groups(self, ftr):
        ftr.set_result(True)
        return
        print ('check groups')
        self.cl = self.nntp()
        for group in self.groups.split(','):
            print ('check {}'.format(group))
            self.cl.group(group)
        ftr.set_result(True)

    def end(self):
        self.daemon.end()
        

    def test_all(self):
        loop = asyncio.get_event_loop()
        try:
            for func in (self.nntp_check_caps, self.nntp_check_groups):
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
            

