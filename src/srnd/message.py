#
# message.py
# 

import logging
import os

from binascii import unhexlify
from calendar import timegm
from datetime import datetime, timedelta
from email.feedparser import FeedParser
from email.utils import parsedate_tz
from hashlib import sha1, sha512

import nacl.signing

from . import config
from . import sql
from . import util

class Message:
    """
    nntp post message
    """
    
    def __init__(self, article_uid):
        self.log = logging.getLogger('nntp-message')
        assert util.is_valid_article_id(article_uid)
        conf = config.load_config()
        base_dir = conf['store']['base_dir']
        self.fname = os.path.join(base_dir, article_uid)
        self.message_id = article_uid
        self.hash_message_uid = sha1(article_uid.encode('ascii')).hexdigest()
        self.identifier = self.hash_message_uid[:10]
        self.subject = 'None'
        self.sender = 'Anonymous'
        self.email = ''
        self.parent = ''
        self.path = ''
        self.sent = 0
        self.groups = list()
        self.sage = False
        self.sig = None
        self.pubkey = ''
        self.image_name = ''
        self.thumb_name = ''
        self.image_link = ''
        self.thumb_link = ''
        self.image_hash = ''
        self.filename = ''
        self.message = bytearray()
        self._result = None


    def dicts(self):
        ret = list()
        for group in self.groups:
            ret.append({
                'message_id': self.message_id,
                'messgae': self.message,
                'subject': self.subject,
                'name': self.sender,
                'posted_at': self.sent,
                'pubkey': self.pubkey,
                'sig': self.sig,
                'references': self.parent,
                'filename' : self.image_name,
                'email': self.email,
                'imagehash': self.image_hash,
                'posthash' : self.hash_message_uid,
                'newsgroup' : group
            })
        return ret

    def save(self, con):
        """
        save to database
        """
        if self.message_id:
            con.execute(
                sql.articles.insert(),
                self.dicts())
            vals = list()
            for group in self.groups:
                count = con.execute(
                    sql.select([sql.newsgroups.c.article_count]).where(
                        sql.newsgroups.c.name == group)
                ).fetchone()[0]
                vals.append({
                    'post_id': count + 1, 
                    'newsgroup' : group, 
                    'article_id': self.message_id})
                con.execute(sql.newsgroups.update().values(
                    updated = datetime.utcnow(),
                    article_count = count + 1).where(
                        sql.newsgroups.c.name == group)
                        )
            con.execute(
                sql.article_posts.insert(), vals)
        else:
            raise Exception("article invalid, no article_id")

    def load(self, f=None):
        """
        load message
        """
        if f is None:
            with open(self.fname) as f:
                return self._load(f)
        else:
            return self._load(f)

    def _check_header(self, hdr):
        """
        check previously loaded line for header
        """
        hdr += ':'
        return self._lline.startswith(hdr)
        
    def _splitit(self):
        """
        some kinda dark srnd magic
        """
        return self._line.split(' ', 1)[1][:-1]


    def _load(self, fd):
        """
        load from file descriptor
        """
        hdr_found = False
        #_parser = FeedParser()
        # load headers
        self._line = fd.readline()
        while len(self._line) > 0:
            #_parser.feed(self._line)
            self._lline = self._line.lower()
            if self._check_header('subject'):
                # parse subject header
                self.subject = self._splitit()
            elif self._check_header('path'):
                self.path = self._line[6:]
            elif self._check_header('date'):
                # parse date header
                self.sent = self._splitit()
                sent_tz = parsedate_tz(self.sent)
                if sent_tz:
                    offset = 0
                    if sent_tz[-1]: offset = sent_tz[-1]
                    self.sent = timegm((datetime(*sent_tz[:6]) - timedelta(seconds=offset)).timetuple())
                else:
                    self.sent = int(time.time())
            elif self._check_header('from'):
                # from / email header
                parts = self._splitit().split(' <', 1)
                self.sender = parts[0]
                if len(parts) > 1:
                    self.email = parts[1].replace('>','')
            elif self._check_header('references'):
                # references header
                self.parent = self._line[:-1].split(' ')[1]
            elif self._check_header('newsgroups'):
                # newsgroups header
                group_in = self._line[:-1].split(' ', 1)[1]
                if ';' in group_in:
                    for group in group_in.split(';'):
                        if group.startswith('overchan.'):
                            self.groups.append(group)
                else:
                    self.groups.append(group_in)
            elif self._check_header('x-sage'):
                self.sage = True
            elif self._check_header('x-pubkey-ed25519'):
                self.pubkey = self._lline[:-1].split(' ',1)[1]
            elif self._check_header('x-signature-ed25519-sha512'):
                self.sig = self._lline[:-1].split(' ',1)[1]
            elif self._line == '\n':
                hdr_found = True
                break
            self._line = fd.readline()
        if not hdr_found:
            self.log.error('{} malformed article'.format(self.message_id))
            return False
        if self.sig is not None and self.pubkey != '':
            self.log.info('got signature with length {} and content {}'.format(len(self.sig), self.sig))
            self.log.info('got public key with length {} and content {}'.format(len(self.pubkey), self.pubkey))
            if len(self.sig) != 128 or len(self.pubkey) != 64:
                self.pubkey = ''
        # verify sig
        if self.pubkey != '':
            bodyoffset = fd.tell()
            hasher = sha512()
            oldline = None
            for line in fd:
                if oldline:
                    hasher.update(oldline.encode('utf-8'))
                oldline = line.replace("\n", "\r\n")
            hasher.update(oldline.replace("\r\n", "").encode('utf-8'))
            dg = hasher.digest()
            fd.seek(bodyoffset)
            try:
                self.log.info('trying to validate signature...')
                nacl.signing.VerifyKey(
                    unhexlify(
                        self.pubkey
                    )
                ).verify(
                    dg,
                    unhexlify(
                        self.sig
                    )
                )
                self.log.info('valid signature :3')
            except Exception as e:
                self.log.error('failed to validate: {}'.format(e))
        return True
        # read body
        #_parser.feed(fd.read())
        #self._result = _parser.close()
        #del _parser
        
