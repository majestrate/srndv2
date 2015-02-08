#
# nntp.py
#
import asyncio
import logging

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
    def send(self, data):
        """
        send them arbitrary data
        """
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
                yield from self.send(line+'\r\n')
        while self._run: 
            line = yield from self.r.readline()
            commands = None
            if self.state == 'multiline':
                self._lines.append(line)
            commands = line.split()

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
                    
