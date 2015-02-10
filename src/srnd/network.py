#
# network.py
#
from . import config
from . import nntp
from . import storage
import asyncio
import logging


class NNTPD:
    """
    nntp daemon
    """

    def __init__(self, daemon_conf, feed_config, store_config):
        """
        pass in valid config from config parser
        """
        self.log = logging.getLogger('nntpd')
        self.bindhost = daemon_conf['bind_host']
        self.bindport = daemon_conf['bind_port']
        # TODO: move to use as parameter
        self.feed_config = feed_config
        self.default_feed_policy = nntp.FeedPolicy(self.feed_config['default'].keys())
        self.article_store = storage.FileSystemArticleStore(store_config)


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
        
