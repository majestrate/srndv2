#
# network.py
#
from . import nntp
import asyncio
import logging


class NNTPD:
    """
    nntp daemon
    """

    def __init__(self, conf):
        """
        pass in valid config from config parser
        """
        self.log = logging.getLogger('nntpd')
        self.bindhost = conf['bind_host']
        self.bindport = conf['bind_port']


    def start(self):
        """
        start daemon
        bind to address given via config
        """
        self.loop = asyncio.get_event_loop()
        coro = asyncio.start_server(self.on_ib_connection, self.bindhost, self.bindport, loop=self.loop)
        self.serv = self.loop.run_until_complete(coro)
        print('nntpd serving on {}'.format(self.serv.sockets[0].getsockname()))
        
    def on_ib_connection(self, r, w):
        """
        we got an inbound connection
        """
        self.log.info('inbound connection made')
        conn = nntp.Connection(self, r, w, True)
        asyncio.async(conn.run())

    def end(self):
        """
        end daemon activity
        """
        self.serv.close()
        self.loop.run_until_complete(self.serv.wait_closed())
        
