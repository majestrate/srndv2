#
# main.py
#

from . import config
from . import network
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
    logging.basicConfig(level=lvl)

    srnd_conf = conf['srnd']
    daemon = network.NNTPD(srnd_conf)
    daemon.run()
    
