#
# config.py
#
import configparser
import logging
import os

def load_config(fname='srnd.ini'):
    """
    load user configuration
    """
    
    config = configparser.ConfigParser()
    if not os.path.exists(fname):
        # generate default config
        config['log'] = dict()
        config['log']['level'] = 'info'
        
        config['database'] = dict()
        config['database']['url'] = 'sqlite:///test.db'

        config['store'] = dict()
        config['store']['base_dir'] = './articles/'

        config['srnd'] = dict()
        config['srnd']['instance_name'] = '{}.srndv2.tld'.format(os.environ['USER'])
        config['srnd']['bind_host'] = '::1'
        config['srnd']['bind_port'] = '1199'
        config['srnd']['sync_on_start'] = '1'
        with open(fname, 'w') as f:
            config.write(f)
    config.read(fname)
    return config


def load_feed_config(fname='feeds.ini'):
    """
    load outfeed config
    """
    
    config = configparser.ConfigParser()
    if not os.path.exists(fname):
        # generate default config
        config['feed-some.onion:119'] = dict()
        config['feed-some.onion:119']['proxy-type'] = 'socks4a'
        config['feed-some.onion:119']['proxy-host'] = '127.0.0.1'
        config['feed-some.onion:119']['proxy-port'] = '9050'
        config['some.onion:119'] = dict()
        config['some.onion:119']['overchan.*'] = '1'
        config['some.onion:119']['ano.paste'] = '0'
        config['some.onion:119']['ctl'] = '1'
        

        with open(fname, 'w') as f:
            config.write(f)
    config.read(fname)
    return config
