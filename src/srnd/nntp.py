#
# nntp.py
#
import asyncio
import logging
import re


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
            commands = None
            if self.state == 'multiline':
                self._lines.append(line)
            commands = line.strip('\r\n').split()
            self.log.debug('commands {}'.format(commands))
            if self.ib: # inbound
                if commands: # we are wanting a command
                    if commands[0] == 'CAPABILITIES': # send capabilities
                        for cap in self.caps:
                            yield from self.send(cap + '\r\n')
                        yield from self.send('.\r\n')
            else:
                if commands:
                    if self.state == 'initial':
                        if commands[0] == '200':
                            # request caps
                            self.send('CAPABILITIES\r\n')
                            self.state = 'multiline'
                    
