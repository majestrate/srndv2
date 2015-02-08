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

        config['srnd'] = dict()
        config['srnd']['instance_name'] = '{}.srndv2.tld'.format(os.environ['USER'])
        config['srnd']['bind_host'] = '::1'
        config['srnd']['bind_port'] = '1199'
        with open(fname, 'w') as f:
            config.write(f)
    config.read('srnd.ini')
    return config


def load_outfeeds(fname='outfeeds.ini'):
    """
    load outfeed config
    """
