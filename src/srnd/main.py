#
# main.py
#

from . import config
from . import network
from . import sql
import asyncio
import logging

def main():
    """
    run srnd
    """
    conf = config.load_config()
    log = conf['log']
    if log['level'].lower() == 'debug':
        lvl = logging.DEBUG
    else:
        lvl = logging.INFO
    logging.basicConfig(level=lvl, format=' %(levelname)s %(asctime)s %(filename)s:%(lineno)d %(name)s %(message)s')

    srnd_conf = conf['srnd']
    store_conf = conf['store']
    feed_conf = config.load_feed_config()
    daemon = network.NNTPD(srnd_conf, feed_conf, store_conf)
    sql.create()
    daemon.start()
    loop = asyncio.get_event_loop()
    try:
        loop.run_forever()
    finally:
        daemon.end()
        loop.close()

