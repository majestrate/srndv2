#
# nntp.py
#
import asyncio
import logging
import os
import re

from . import util

class PolicyRule:
    """
    string / regexp based rule matcher
    """

    def __init__(self, rule):
        if rule[0] == '*' or rule == '!*' or rule == '!':
            raise Exception('invalid rule: {}'.format(rule))

        self.is_re = '*' in rule
        self._inv = rule[0] == '!'
        if self._inv:
            rule = rule[1:]
        self.rule = rule.lower()
        if self.is_re:
            rule = rule.replace('.', '\\.')
            self.rule = re.compile(rule)
            

    def _re_check(self, newsgroup):
        return self.rule.match(newsgroup) is not None

    def allows_newsgroup(self, newsgroup):
        """
        check if this rule allows a newsgroup
        """
        res = False
        if self.is_re:
            res = self._re_check(newsgroup)
        else:
            res = newsgroup.lower() == self.rule
        if self._inv:
            return not res
        return res

class FeedPolicy:
    """
    dictactes what groups are carried and accepted
    """

    def __init__(self, rule_strs):
        self.rules = list()
        for rule in rule_strs:
            self.rules.append(PolicyRule(rule))
        
    def allow_newsgroup(self, newsgroup):
        for rule in self.rules:
            if rule.match(newsgroup):
                return True

class Connection:
    """
    nntp connection handler
    """

    caps = (
        '101 I support some things',
        'VERSION 2',
        'IMPLEMENTATION srndv2 better overchan nntpd v0.1',
        'POST',
        'IHAVE',
        'STREAMING'
    )

    welcome = (
        '200 ayyyy lmao overchan nntpd, post it faget',
    )

    def __init__(self, daemon, r, w, incoming=None):
        """
        pass in a reader and writer that is connected to an endpoint
        no data is sent or received before this
        """
        self.log = logging.getLogger('nntp-connection')
        self.daemon = daemon
        self.r, self.w = r, w
        self.ib = incoming is True
        self.state = 'initial'
        self._run = False
        self._lines = list()

    @asyncio.coroutine
    def sendline(self, line):
        yield from self.send(line)
        yield from self.send(b'\r\n')
 
    @asyncio.coroutine
    def send(self, data):
        """
        send them arbitrary data
        """
        self.log.debug('send data: {}'.format(data))
        if not isinstance(data, bytes):
            data = data.encode('ascii')
        self.w.write(data)
        yield from self.w.drain()
        data = None

    def enable_stream(self):
        """
        enable streaming mode
        """
        pass            

    @asyncio.coroutine
    def handle_CAPABILITIES(self, args):
        """
        handle capacities command
        """
        for cap in self.caps:
            yield from self.send(cap + '\r\n')
        yield from self.send('.\r\n')


    @asyncio.coroutine
    def handle_GROUP(self, args):
        # TODO: handle group command
        yield from self.send_response(411, 'no such news group')
        


    @asyncio.coroutine
    def handle_QUIT(self, args):
        yield from self.send_response(205, 'kthnxbai')
        self.w.close()
        
    @asyncio.coroutine
    def handle_MODE(self, args):
        """
        handle MODE command
        currently only supports STREAM
        """
        if args[0] == 'STREAM':
            self.enable_stream()
            yield from self.send_response(203, 'stream as desired yo')
        else:
            yield from self.send_response(501, 'Unknown MODE option')

    def handle_CHECK(self, args):
        """
        handle CHECK command
        checks if article exists
        """
        aid = args[0]
        if self.daemon.store.article_banned(aid):
            yield from self.send_response(437, '{} this article is banned'.format(aid))
        elif self.daemon.store.has_article(args):
            yield from self.send_response(435, '{} we have this article'.format(aid))
        else:
            yield from self.send_response(238, '{} article wanted plz gib'.format(aid))
            
    @asyncio.coroutine
    def handle_TAKETHIS(self, args):
        """
        handle TAKETHIS command
        takes 1 article
        """
        if util.is_valid_article_id(args[0]):
            with self.daemon.store.open_article(args[0]) as f:
                line = yield from self.r.readline()
                while line != b'.\r\n':
                    line = line.replace(b'\r', b'')
                    f.write(line)
                    try:
                        line = yield from self.r.readline()
                    except ValueError as e:
                        self.log.error('bad line for article {}: {}'.format(args[0], e))
            self.log.info("recv'd article {}".format(args[0]))
            yield from self.send_response(239, 'article transfered okay woot')
        else:
            yield from self.send_response(439, 'article rejected gtfo')
        
    @asyncio.coroutine
    def handle_IHAVE(self, args):
        """
        handle IHAVE command
        """
        if self.daemon.has_article(args[0]):
            yield from self.send_response(435, 'article not wanted do not send it')
        else:
            yield from self.send_response(335, 'send article. End with <CR-LF>.<CL-LF>')
            if util.is_valid_article_id(args[0]):
               with self.daemon.store.open_article(args[0]) as f:
                    line = yield from self.r.readline()
                    while line != b'':
                        f.write(line)
                        f.write(b'\r\n')
                    line = yield from self.r.readline()
                    if line != b'.\r\n':
                        self.log.warn('expected end of article but did not get it')
            else:
               yield from self.send_response(437, 'article rejected, invalid id')
                    

    @asyncio.coroutine
    def send_response(self, code, message):
        """
        send an error respose
        """
        yield from self.send('{} {}\r\n'.format(code, message))

    @asyncio.coroutine
    def run(self):
        """
        run the connection mainloop
        """
        self._run = True

        ##
        ## TODO: don't be lazy and use srndv1's nntp logic
        ## 

        if self.ib: # send initial welcome banner if inbound
            for line in self.welcome:
                yield from self.sendline(line)

        while self._run: 

            line = yield from self.r.readline()
            line = line.decode('ascii')

            self.log.debug('got line: {}'.format(line))

            if len(line) == 0:
                self._run = False
                break

            commands = line.strip('\r\n').split()

            self.log.debug('commands {}'.format(commands))

            if commands:
                meth = 'handle_{}'.format(commands[0].upper())
                args = len(commands) > 1 and commands[1:] or list()
                if hasattr(self, meth):
                    yield from getattr(self, meth)(args)
                else:
                    yield from self.send_response(503, '{} not implemented'.format(commands[0]))
