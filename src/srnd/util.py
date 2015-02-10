#
# util.py
#

import os
import string

def sanitize_filename(fname):
    """
    sanitize a filename so it's safe to use
    """
    return fname.replace('.','_').replace('/','_')

def is_valid_article_id(aid):
    """
    return false if article id is disformed
    """
    if isinstance(aid, bytes):
        aid = aid.decode('ascii')
    for ch in aid[1:-1]:
        if ch == '<' or ch == '>' or ch == ' ':
            return False
    if '@@' in aid:
        return False
    return aid[0] == '<' and aid[-1] == '>' and aid.index('@') > 1 and '/' not in aid and '..' not in aid

def ensure_dir(dirname):
    if not os.path.exists(dirname):
        os.mkdir(dirname)
