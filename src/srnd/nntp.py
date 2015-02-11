#
# nntp.py
#
import asyncio
import logging
import os
import re
import traceback

from . import message
from . import sql
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
        'READER',
        'XSECRET',
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
        self.db = sql.SQL()
        self.db.connect()
        self.group = None
        self.mode = None
        self.post = False
        self.authorized = True

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
            data = data.encode('utf-8')
        self.w.write(data)
        try:
            _ = yield from self.w.drain()
        except Exception as e:
            self.log.error(e)
            self.close()
        data = None

    def enable_stream(self):
        """
        enable streaming mode
        """
        self.state = 'stream'           

    def enable_reader(self):
        """
        enable reader mode
        """
        self.state = 'reader'

    @asyncio.coroutine
    def handle_XOVER(self, args):
        """
        todo: implement this
        """
        if self.group is None:
            yield from self.send_response(412, 'no news group selected')
        else:
            yield from self.send_response(420, 'not implementing this now')
    
    @asyncio.coroutine
    def handle_POST(self, args):
        """
        handle posting via POST command
        """
        if self.authorized:
            yield from self.send_response(340, 'send article to be posted. End with <CR-LF>.<CR-LF>')
            article_id = self.daemon.generate_id()
            line = ''
            with self.daemon.store.open_article(article_id) as f:
                while True:
                    line = yield from self.readline()
                    self.log.debug('read line: {}'.format(line))
                    if line == b'.\r\n':
                        break
                    if line.startswith(b'Path:'):
                        # inject path header
                        line = b'Path: '+self.daemon.instance_name.encode('ascii') + b'!' + line[6:] 
                    line = line.replace(b'\r\n', b'\n')
                    f.write(line)
            with self.daemon.store.open_article(article_id, True) as f:
                m = message.Message(article_id)
                m.load(f)
                self.daemon.store.save_message(m)
            yield from self.send_response(240, 'article posted, boohyeah')
        else:
            yield from self.send_response(440, 'posting not allowed')

    @asyncio.coroutine
    def handle_CAPABILITIES(self, args):
        """
        handle capacities command
        """
        for cap in self.caps:
            yield from self.send(cap + '\r\n')
        yield from self.send('.\r\n')

    
    @asyncio.coroutine
    def handle_HEAD(self, args):
        res = self.db.connection.execute(
            sql.select([sql.articles.c.message_id]).where(
                sql.articles.c.post_id == args[0] and sql.articles.c.newsgroup == self.group)).fetchone()
        if res:
            article_id = res[0]
            yield from self.send_response(221, '{} {} headers get, text follows'.format(args[0], article_id))
            with self.daemon.store.open_article(article_id, True) as f:
                while True:
                    line = f.readline()
                    if line == '\r\n' or line == '\n' or len(line) == 0:
                        yield from self.send(b'\r\n.\r\n')
                        return
                    else:
                        yield from self.send(line)
        else:
            yield from self.send_response('432', 'no suck article')

    @asyncio.coroutine
    def handle_ARTICLE(self, args):
        res = self.db.connection.execute(
            sql.select([sql.articles.c.message_id]).where(
                sql.articles.c.post_id == args[0] and sql.articles.c.newsgroup == self.group)).fetchone()
        if res:
            article_id = res[0]
            yield from self.send_response(220, '{} {} atricle get, text follows'.format(args[0], article_id))
            with self.daemon.store.open_article(article_id, True) as f:
                while True:
                    line = f.readline()
                    if len(line) == 0:
                        yield from self.send(b'\r\n.\r\n')
                        return
                    else:
                        yield from self.send(line)
        else:
            yield from self.send_response('432', 'no suck article')
        
    @asyncio.coroutine
    def handle_GROUP(self, args):
        if self.state == 'reader' and self.daemon.store.has_group(args[0]):
            num, p_min, p_max = self.daemon.store.get_group_info(args[0])
            yield from self.send_response(211,'{} {} {} {}'.format(num, p_min, p_max, args[0]))
            self.group = args[0]
        else:
            yield from self.send_response(411, 'no such news group')
            
    @asyncio.coroutine
    def handle_LIST(self, args):
        if len(args) > 0 and args[0].lower() == 'overview.fmt':
            yield from self.send_response(503, 'wont do this sorry')
        elif self.state == 'reader':
            yield from self.send_response(215, 'list of newsgroups ahead')
            for group in self.daemon.store.get_all_groups():
                _, first, last = self.daemon.store.get_group_info(group)
                posting = 'y'
                yield from self.send('{} {} {} {}\r\n'.format(group, last, first, posting))
            yield from self.send(b'.\r\n')
        else:
            yield from self.send_response(500, 'nope')

    @asyncio.coroutine
    def handle_QUIT(self, args):
        yield from self.send_response(205, 'kthnxbai')
        self.close()
        
    @asyncio.coroutine
    def handle_MODE(self, args):
        """
        handle MODE command
        currently only supports STREAM
        """
        if args[0] == 'STREAM':
            self.enable_stream()
            yield from self.send_response(203, 'stream as desired yo')
        elif args[0] == 'READER':
            self.enable_reader()
            yield from self.send_response(200,'Reader mode, reading all fine')
        else:
            yield from self.send_response(501, 'Unknown MODE option')

    def handle_CHECK(self, args):
        """
        handle CHECK command
        checks if article exists
        """
        aid = args[0]
        if self.daemon.store.article_banned(aid) or not util.is_valid_article_id(aid):
            yield from self.send_response(437, '{} this article is banned'.format(aid))
        elif self.daemon.store.has_article(aid):
            yield from self.send_response(435, '{} we have this article'.format(aid))
        else:
            yield from self.send_response(238, '{} article wanted plz gib'.format(aid))

    def handle_XSECRET(self, args):
        if len(args) == 2:
            if self.daemon.store.check_user_login(args[0], args[1]):
                self.authorized = True
                yield from self.send_response(290, 'passwor for {} allowed'.format(args[0]))
        else:
            yield from self.send_response(481, 'Invalid login')

            

    @asyncio.coroutine
    def handle_TAKETHIS(self, args):
        """
        handle TAKETHIS command
        takes 1 article
        """
        has = self.daemon.store.has_article(args[0])
        with self.daemon.store.open_article(args[0]) as f:
            line = yield from self.r.readline()
            while line != b'.\r\n':
                line = line.replace(b'\r', b'')
                if not has:
                    if line.startswith(b'Path:'):
                        # inject path header
                        line = b'Path: '+self.daemon.instance_name.encode('ascii') + b'!' + line[6:] 
                    f.write(line)
                try:
                    line = yield from self.r.readline()
                except ValueError as e:
                    self.log.error('bad line for article {}: {}'.format(args[0], e))
        if not has:            
            with self.daemon.store.open_article(args[0], True) as f:
                m = message.Message(args[0])
                m.load(f)
                self.daemon.store.save_message(m)
                
        self.log.info("recv'd article {}".format(args[0]))
        if self.daemon.store.has_article(args[0]):
            yield from self.send_response(239, args[0])
        else:
            yield from self.send_response(439, args[0])
        
    @asyncio.coroutine
    def send_response(self, code, message):
        """
        send an error respose
        """
        yield from self.send('{} {}\r\n'.format(code, message))
    
    @asyncio.coroutine
    def send_article(self, article_id):
        """
        send an article
        return True on success
        """
        if self.ib:
            self.log.debug('do not send on inbound connection')
            return
        else:
            self.log.info('send article {}'.format(article_id))
            _ = yield from self.sendline('POST')
            self.post = article_id

    def close(self):
        if self in self.daemon.feeds:
            self.daemon.feeds.remove(self)
        self.w.close()
        
    @asyncio.coroutine
    def readline(self):
        self.log.debug('readline')
        d = yield from self.r.readline()
        return d

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
                _ = yield from self.sendline(line)
        else:
            try:
                line = yield from self.readline()
                if line is None:
                    self.log.error('no data read')
                    self._run = False
                    self.close()
                    return
                self.log.debug(line)
                line = line.decode('utf-8')
                if not line.startswith('200 '):
                    self.log.error('cannot post')
                    self.sendline('QUIT')
                    _ = yield from self.readline()
                    self.close()
                    return
                # send caps
                _ = yield from self.sendline('CAPABILITIES')

                line = yield from self.readline()
                caps = list()
                while len(line) > 0 and line != b'.\r\n':
                    caps.append(line.decode('utf-8')[:-2])
                    line = yield from self.readline()
                    self.log.debug('got line {}'.format(line))
                self.log.debug('endcaps {}'.format(caps))
                if 'STREAMING' in caps:
                    _ = yield from self.sendline('MODE STREAM')
                    resp = yield from self.readline()
                    resp = resp.decode('utf-8')
                    if resp.startswith('203 '):
                        self.log.info('enable streaming')
                        self.enable_stream()
            except Exception as e:
                self.log.error(traceback.format_exc())
                return
        while self._run: 
            try:
                line = yield from self.readline()
                line = line.decode('utf-8')
            except Exception as e:
                self.log.error(traceback.format_exc())
                self._run = False
                break

            self.log.debug('got line: {}'.format(line))

            if len(line) == 0:
                self._run = False
                break
            if self.post:
                if line.startswith('340 '):
                    self.log.debug('posting...{}'.format(line))
                    with self.daemon.store.open_article(self.post, True) as f:
                        while True:
                            line = f.readline()
                            if len(line) == 0:
                                self.send(b'.\r\n')
                                self.post = None
                                break
                            _ = yield from self.send(line)

                else:
                    self.log.error(line)
            else:
                commands = line.strip('\r\n').split()

                self.log.debug('commands {}'.format(commands))

                if commands:
                    meth = 'handle_{}'.format(commands[0].upper())
                    args = len(commands) > 1 and commands[1:] or list()
                    if hasattr(self, meth):
                        yield from getattr(self, meth)(args)
                    else:
                        yield from self.send_response(503, '{} not implemented'.format(commands[0]))
