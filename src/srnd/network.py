#
# network.py
#
from . import config
from . import nntp
from . import storage
from . import util
import asyncio
import logging
import os
import queue
import struct
import time


from hashlib import sha1

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
        self.name = daemon_conf['instance_name']
        self.instance_name = self.name
        # TODO: move to use as parameter
        self.feed_config = feed_config
        self.store = storage.FileSystemArticleStore(self, store_config)
        self.feeds = list()

    def got_article(self, article_id, groups):
        """
        a feed got an article
        """
        self.log.debug('article added {}'.format(article_id))
        if groups:
            for feed in self.feeds:
                for group in groups:
                    if feed.policy.allow_newsgroup(group):
                        if feed.article_queued(article_id):
                            self.log.debug('article queued already')
                        else:
                            feed.queue_send_article(article_id)
        else:
            self.log.warning('no newsgroups for {}'.format(article_id))
        
    def generate_id(self):
        now = int(time.time())
        id = sha1(os.urandom(8)).hexdigest()[:10]
        return '<{}.{}@{}>'.format(now, id, self.name)

    def start(self):
        """
        start daemon
        bind to address given via config
        """
        self.loop = asyncio.get_event_loop()
        coro = asyncio.start_server(self.on_ib_connection, self.bindhost, self.bindport, loop=self.loop)
        self.serv = self.loop.run_until_complete(coro)
        print('nntpd serving on {}'.format(self.serv.sockets[0].getsockname()))
        self.create_outfeeds()

    def create_outfeeds(self):
        feeds = dict()
        for key in self.feed_config:
            if key.startswith('feed-'):
                cfg = self.feed_config[key]
                host, port = util.parse_addr(key[5:])
                key = '{}:{}'.format(host,port)
                feeds[key] = dict()
                feeds[key]['settings'] = cfg
                feeds[key]['config'] = self.feed_config[key]
        for key in feeds:
            feed = Outfeed(key, self, feeds[key])
            asyncio.async(feed.run())
            self.feeds.append(feed)

    def on_ib_connection(self, r, w):
        """
        we got an inbound connection
        """
        self.log.info('inbound connection made')
        conn = nntp.Connection(self, self.default_feed_policy, r, w, True)
        asyncio.async(conn.run())
        #self.feeds(

    def end(self):
        """
        end daemon activity
        """
        self.serv.close()
        self.loop.run_until_complete(self.serv.wait_closed())


class Outfeed:

    def __init__(self, addr, daemon, conf):
        self.addr = util.parse_addr(addr)
        self.daemon = daemon
        self.name = '%s-%s' % self.addr
        self.settings = conf['settings']
        self.policy = nntp.FeedPolicy.from_conf(conf['config'])
        self.log = logging.getLogger('outfeed-{}'.format(self.name))
        self.feed = None
        


    @asyncio.coroutine
    def add_article(self, article_id):
        self.log.debug('add article: {}'.format(article_id))
        if self.feed:
            yield from self.feed.send_article(article_id)

    @asyncio.coroutine
    def proxy_connect(self, proxy_type):
        if proxy_type == 'socks4a':
            phost = self.settings['proxy-host']
            pport = int(self.settings['proxy-port'])
            r, w = yield from asyncio.open_connection(phost, pport)
            # socks 4a handshake
            req = b'\x04\x01' + struct.pack('>H', self.addr[1]) + b'\x00\x00\x00\x01srndv2\x00' + self.addr[0].encode('ascii') +b'\x00'
            self.log.debug('connect out... {}'.format(req))
            w.write(req)
            _ = yield from w.drain()
            data = yield from r.readexactly(8)
            success = chr(data[1]) == '\x5a'
            self.log.debug('got handshake sucess {}'.format(success))
            if success:
                return r, w
            w.close()
            yield from asyncio.sleep(1)
            return None
        elif proxy_type == 'None' or proxy_type is None:
            try:
                r ,w = yield from asyncio.open_connection(self.addr[0], self.addr[1])
            except Exception as e:
                self.log.error('cannot connect: {}'.format(e))
                yield from asyncio.sleep(1)
                return None
            else:
                return r, w
        else:
            self.log.error('proxy type not supported: {}'.format(proxy_type))

    @asyncio.coroutine
    def connect(self):
        self.log.info('attempt connection')
        if 'proxy-type' in self.settings:
            ptype = self.settings['proxy-type']
            pair = yield from self.proxy_connect(ptype)
            if pair:
                return pair[0], pair[1]
        else:
            r, w = yield from asyncio.open_connection(self.addr[0], self.addr[1])
            return r, w    

    @asyncio.coroutine
    def run(self):
        self._run = True
        while self._run:
            if self.feed is None:
                pair = yield from self.connect()
                if pair:
                    r, w = pair
                    self.log.info('connected')
                    self.feed = nntp.Connection(self.daemon, r, w, name=self.name)
                    asyncio.async(self.feed.run())
                else:
                    self.log.debug('did not connect')
            else:
                _ = yield from asyncio.sleep(1)
