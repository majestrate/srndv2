#
# overchan.py
#
# frontend reference implementation
#

__doc__ = """
frontend reference implementation
"""
from . import database
from . import models
from . import srndapi
from . import util
from . import thumbs

from tornado import web

import time
import logging
import os
import base64
import threading

class WebHandler(web.RequestHandler):
    def initialize(self, srndapi):
        self.api = srndapi

class ModHandler(WebHandler):

    def post(self):
        """
        handle moderation request
        """
        message_id = self.get_argument("message_id")

        #
        # action parameter takes the following possible values right now
        #
        # delete:post    -- delete the file and the post itself
        # delete:thread  -- delete the thread and all posts and files related (message_id must be for a root post)
        #
        action = self.get_argument("action")
        action = action.lower()
        
        if action.startswith("delete:"):
            subaction = action[action.index(":"):]
            if subaction == 'thread':
                hook = self.api.delete_thread
            elif subaction == 'post':
                hook = self.api.delete_post
            else:
                self.write("invalid delete sub-action: {}".format(subaction))
                return
            result = hook(message_id)
            self.write(hook)
        else:
            self.write("invalid action: {}".format(action))
            
            
class PostHandler(WebHandler):

    def post(self):
        """
        handle new post from user
        """
        newsgroup = self.get_argument("newsgroup")
        parent = self.get_argument("parent", default="")
        name = self.get_argument("name", default="Anonymous")
        email =  self.get_argument("email", default="anon@nowhere.tld")
        subject = self.get_argument("subject", default="None")
        message = self.get_argument("message")
        msg = self.api.post(newsgroup, parent, name, email, subject, message)
        self.write(msg)

class Frontend(srndapi.SRNdAPI):
    """
    srndv2 overchan+postman reference implementation
    """
    def __init__(self, loop, name):
        srndapi.SRNdAPI.__init__(self, loop, name+".tld", name)
        self.name = name
        self.sql = database.open()

    def delete_thread(self, msgid):
        """
        delete a thread recursively given a root post
        """

    def delete_post(self, msgid):
        """
        delete a post and file attachment
        """
 
        
    def got(self, obj):
        """
        we got an incoming object
        """
        if obj["Please"] == "post":
            self.got_post(obj)
            
    def put_file(self, post_id, file_obj):
        """
        put a file onto the disk
        """
        now = time.time() 
        # make filenames
        img_fname = os.path.join("img", "{}{}".format(now, file_obj["Extension"]))
        self.log.info("putfile {}".format(img_fname))
        d = file_obj["Data"]
        
        # decode and save
        d = base64.b64decode(d)
        with open(img_fname, 'w') as f:
            f.write(d)
        # insert image metadata into database
        database.files.insert().values([
            {
                "filename": file_obj["Name"],
                "filepath": img_fname,
                "parent": post_id
            }
        ])
        # check for imagesx
        if file_obj["Mime"].lower().startswith("image/"):
            # for non gif images, make a thumbnail
            if file_obj["Extension"].lower() != '.gif':
                thm_fname = os.path.join("thm", "{}.jpg".format(now))
                # render thumbnail in other thread
                threading.Thread(target=thumbs.render, args=(img_fname, thm_fname)).start()
            else:
                # write gif as thumbnail
                with open(os.path.join("thm", "{}.gif".format(now)), 'w') as f:
                    f.write(d)
        
    def got_post(self, obj):
        """
        we got a post
        """
        self.log.info("we got a post")
        postid = obj['MessageID']
        post = models.Article(postid)
        post.newsgroup = obj['Newsgroup']
        post.parent = obj['Reference']
        path = obj["Path"]
        post.frontend = path[:path.rindex("!")]
        post.comment = obj["Message"]
        post.subject = obj["Subject"]
        post.save()
        
        if obj['Attachments']:
            for f in obj['Attachments']:
                if f['Extension'] != '':
                    self.put_file(obj['MessageID'] ,f)
        

    def find_thread(self, msgid):
        """
        find a thread with the root post given the hashed post id
        """
        
        
    def sync(self, newsgroups=None):
        if newsgroups is None:
            newsgroups = list()
            self.log.info('sync all groups')
        else:
            self.log.info('synching {} groups'.format(len(newsgroups)))
        self.please('sync', newsgroups=newsgroups)
        
    def genID(self):
        return '<{}.{}@{}>'.format(util.random_string(10), util.now(), self.name)
        
    def post(self, newsgroup, parent, name, email, subject, comment, key=None):
        """
        handle a post from flask
        """
        obj = dict()
        if not newsgroup.startswith('overchan.'):
            return 'invalid newsgroup'
        obj['MessageID'] = self.genID()
        obj['Newsgroup'] = newsgroup
        obj['Reference'] = parent
        obj['Name'] = name
        obj['Email'] = email
        obj['Subject'] = subject
        obj['Comment'] = comment
        obj['Key'] = key
        obj['Posted'] = util.now()
        self.please('post', **obj)
        return "posted"
        
    def register_api(self, api):
        self.log.info('registering with api...')

    def has_thread(self, thread_id):
        pass
