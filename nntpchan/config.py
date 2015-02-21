#
# config.py
#
# configuration get/set
#

import json
import os
import threading

__doc__ = """
configuration get / set mechanisms
"""

_lock = threading.Lock()
_fname = 'nntpchan.json'

def get(attr, fname=_fname):
    """
    get nntpchan config value
    """
    _lock.acquire()
    try:
        with open(fname) as f:
            j = json.load(f)
    finally:
        _lock.release()
    if attr in j:
        return j[attr]
        
def set(attr, val, fname=_fname):
    """
    set nntpchan config value
    """
    _lock.acquire()
    try:
        with open(fname) as f:
            j = json.load(f)
        j[attr] = val
        with open(fname, 'w') as f:
            json.dump(j, f)
    finally:
        lock.release()
        
def _genconf(fname=_fname):
    j = dict()
    j['db_url'] = 'postgres://root:root@localhost/'
    with open(fname, 'w') as f:
        json.dump(j, f)
        
if not os.path.exists(_fname):
    _genconf()