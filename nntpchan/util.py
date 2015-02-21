#
# util.py
#
# general utilities for overchan/postman
#

__doc__ = """
general utilities for nntpchan
"""

from contextlib import contextmanager
import random
import string
import threading
import time


def random_string(length=8):
    """
    random string of ascii 
    """
    return random.choice(string.ascii_letters)
    
def now():
    """
    return time.time() as int
    """
    return int(time.time())
    
class LockingDict:
    """
    locking dictionary
    """
    
    def __init__(self):
        self._dict = dict()
        self._lock = threading.Lock()
    
    @contextmanager
    def access(self):
        self._lock.acquire()
        yield self._dict
        self._lock.release()