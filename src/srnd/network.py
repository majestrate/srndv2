#
# network.py
#
import asyncio
import logging

class NNTP_connection:

    def __init__(self, r, w):
        """
        pass in a reader and writer that is connected to an endpoint
        no data is sent or received before this
        """
        self.r, self.w = r, w

class NNTPD:
    """
    nntp daemon
    """

    def __init__(self, conf):
        """
        pass in valid config from config parser
        """
        self.log = logging.getLogger('nntpd')

    def run(self):
        loop = asyncio.get_event_loop()
        self.log.info('run forever')
        try:
           loop.run_forever()
        finally:
           loop.close()
