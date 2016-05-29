// +build !disable_redis

/*
The MIT License (MIT)

Copyright (c) 2016 ZiRo

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in
all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
THE SOFTWARE.
*/

package database

import (
	"errors"
	"fmt"
	"github.com/majestrate/srndv2/lib/model"
	"github.com/majestrate/srndv2/lib/util"
	"github.com/mcuadros/go-version"
	"gopkg.in/redis.v3"
	"log"
	"math"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const MIN_REDIS_VERSION = "3.0"

// Constants for redis key prefixes
// since redis might be shared among many programs, these are used to avoid conflicts.

const APP_PREFIX = "NNTP::"

//hashes - these store the actual data
// for expample NNTP::Article::1234 stores the data of the article with primary key (message id) 1234

const (
	ARTICLE_PREFIX               = APP_PREFIX + "Article::"
	ARTICLE_POST_PREFIX          = APP_PREFIX + "ArticlePost::"
	ARTICLE_KEY_PREFIX           = APP_PREFIX + "ArticleKey::"
	ARTICLE_NUMBERS_PREFIX       = APP_PREFIX + "ArticleNumbers::"
	HASH_MESSAGEID_PREFIX        = APP_PREFIX + "HashMessageID::"
	ATTACHMENT_PREFIX            = APP_PREFIX + "Attachment::"
	BANNED_GROUP_PREFIX          = APP_PREFIX + "BannedGroup::"
	BANNED_ARTICLE_PREFIX        = APP_PREFIX + "BannedArticle::"
	MOD_KEY_PREFIX               = APP_PREFIX + "ModKey::"
	NNTP_LOGIN_PREFIX            = APP_PREFIX + "Login::"
	ENCRYPTED_ADDRS_PREFIX       = APP_PREFIX + "EncryptedAddrs::"
	ADDRS_ENCRYPTED_ADDRS_PREFIX = APP_PREFIX + "AddrsEncryptedAddrs::"
	ENCRYPTED_IP_BAN_PREFIX      = APP_PREFIX + "EncIPBan::"
	IP_BAN_PREFIX                = APP_PREFIX + "IPBan::"
	IP_RANGE_BAN_PREFIX          = APP_PREFIX + "IPRangeBan::"
)

//keyrings - these can be seen as index
//they hold sets of primary keys to hashes or other keyrings
//to do sorting, they may be weighted as well

const (
	GROUP_POSTTIME_WKR                = APP_PREFIX + "GroupPostTimeWKR"
	GROUP_ARTICLE_POSTTIME_WKR_PREFIX = APP_PREFIX + "GroupArticlePostTimeWKR::"
	GROUP_THREAD_POSTTIME_WKR_PREFIX  = APP_PREFIX + "GroupThreadPostTimeWKR::"
	GROUP_THREAD_BUMPTIME_WKR_PREFIX  = APP_PREFIX + "GroupThreadBumpTimeWKR::"
	GROUP_MOD_KEY_REVERSE_KR_PREFIX   = APP_PREFIX + "GroupModKeysKR::"
	THREAD_POST_WKR                   = APP_PREFIX + "ThreadPostsWKR::"
	ARTICLE_WKR                       = APP_PREFIX + "ArticleWKR"
	THREAD_BUMPTIME_WKR               = APP_PREFIX + "ThreadBumpTimeWKR"
	HEADER_KR_PREFIX                  = APP_PREFIX + "HeaderKR::"
	MESSAGEID_HEADER_KR_PREFIX        = APP_PREFIX + "MessageIDHeaderKR::"
	ARTICLE_ATTACHMENT_KR_PREFIX      = APP_PREFIX + "ArticleAttachmentsKR::"
	ATTACHMENT_ARTICLE_KR_PREFIX      = APP_PREFIX + "AttachmentArticlesKR::"
	IP_RANGE_BAN_KR                   = APP_PREFIX + "IPRangeBanKR"
	IP_ARTICLE_KR_PREFIX              = APP_PREFIX + "AddrArticle::"
	IP_WKR                            = APP_PREFIX + "AddrWKR"
)

type RedisDB struct {
	client *redis.Client
}

var redis_version_extractor = regexp.MustCompilePOSIX(`redis_version:([^\r\n]*)`)

func NewRedisDatabase(host, port, password string) DB {
	var client RedisDB
	var err error
	var info string

	log.Println("Connecting to redis...")

	client.client = redis.NewClient(&redis.Options{
		Addr:        net.JoinHostPort(host, port),
		Password:    password,
		DB:          0, // use default DB
		PoolTimeout: 10 * time.Second,
		PoolSize:    100,
	})

	_, err = client.client.Ping().Result() //check for successful connection
	if err != nil {
		log.Fatalf("cannot open connection to redis: %s", err)
	}

	info, err = client.client.Info("server").Result()
	if err != nil {
		log.Fatalf("cannot open connection to redis: %s", err)
	}

	res := redis_version_extractor.FindStringSubmatch(info)

	if len(res) != 2 {
		log.Fatalf("cannot determin redis version")
	}
	cur_version := res[1]

	if version.Compare(cur_version, MIN_REDIS_VERSION, "<") {
		log.Fatalf("Please upgrade redis. Need version %s. Have version %s.", MIN_REDIS_VERSION, cur_version)
	}

	return client
}

// finalize all transactions
// close database connections
func (self RedisDB) Close() {
	if self.client != nil {
		self.client.Close()
		self.client = nil
	}
}

func (self RedisDB) Ensure() error {
	return nil
}

func (self RedisDB) BanNewsgroup(group string) (err error) {
	_, err = self.client.HMSet(BANNED_GROUP_PREFIX+group, "newsgroup", group, "time_banned", strconv.Itoa(int(util.TimeNow()))).Result()
	return
}

func (self RedisDB) UnbanNewsgroup(group string) (err error) {
	_, err = self.client.Del(BANNED_GROUP_PREFIX + group).Result()
	return
}

func (self RedisDB) NewsgroupBanned(group string) (banned bool, err error) {
	banned, err = self.client.Exists(BANNED_GROUP_PREFIX + group).Result()
	return
}

/*
func (self RedisDB) NukeNewsgroup(group string, store ArticleStore) {
	// get all articles in that newsgroup
	chnl := make(chan ArticleEntry, 24)
	go func() {
		self.GetAllArticlesInGroup(group, chnl)
		close(chnl)
	}()
	// for each article delete it from disk
	for {
		article, ok := <-chnl
		if ok {
			msgid := article.MessageID()
			log.Println("delete", msgid)
			// remove article from store
			fname := store.GetFilename(msgid)
			os.Remove(fname)
			// get all attachments
			for _, att := range self.GetPostAttachments(msgid) {
				// remove attachment
				log.Println("delete attachment", att)
				os.Remove(store.ThumbnailFilepath(att))
				os.Remove(store.AttachmentFilepath(att))
			}
		} else {
			break
		}
	}
	threads, _ := self.client.ZRange(GROUP_THREAD_BUMPTIME_WKR_PREFIX+group, 0, -1).Result()
	for _, t := range threads {
		self.DeleteThread(t)
	}

	mods, _ := self.client.SMembers(GROUP_MOD_KEY_REVERSE_KR_PREFIX + group).Result()
	for _, m := range mods {
		self.client.Del(MOD_KEY_PREFIX + m + "::Group::" + group + "::Permissions")
	}
	self.client.Del(GROUP_MOD_KEY_REVERSE_KR_PREFIX + group)
	self.client.Del(GROUP_ARTICLE_POSTTIME_WKR_PREFIX+group, GROUP_THREAD_POSTTIME_WKR_PREFIX+group, GROUP_THREAD_BUMPTIME_WKR_PREFIX+group) //these should be empty at this point anyway
	self.client.ZRem(GROUP_POSTTIME_WKR, group)
	self.client.ZRem(ARTICLE_NUMBERS_PREFIX+"last", group)github.com/majestrate/srndv2/lib/database

	log.Println("nuke of", group, "done")
}
*/

func (self RedisDB) AddModPubkey(pubkey string) error {
	has, err := self.CheckModPubkey(pubkey)
	if has {
		log.Println("did not add pubkey", pubkey, "already exists")
		return err
	}
	_, err = self.client.SAdd(MOD_KEY_PREFIX+pubkey+"::Group::"+"ctl"+"::Permissions", "login").Result()
	return err
}

func (self RedisDB) GetGroupForMessage(message_id string) (group string, err error) {
	group, err = self.client.HGet(ARTICLE_POST_PREFIX+message_id, "newsgroup").Result()
	return
}

func (self RedisDB) GetPageForRootMessage(root_message_id string) (group string, page int64, err error) {
	group, err = self.GetGroupForMessage(root_message_id)
	if err == nil {
		var index int64
		perpage, _ := self.GetPagesPerBoard(group)
		index, err = self.client.ZRevRank(GROUP_THREAD_BUMPTIME_WKR_PREFIX+group, root_message_id).Result()
		page = int64(math.Floor(float64(index) / float64(perpage)))
	}
	return
}

func (self RedisDB) GetInfoForMessage(msgid string) (root string, newsgroup string, page int64, err error) {
	root, err = self.client.HGet(ARTICLE_POST_PREFIX+msgid, "ref_id").Result()
	if err == nil {
		if root == "" {
			root = msgid
		}
		newsgroup, page, err = self.GetPageForRootMessage(root)
	}
	return
}

func (self RedisDB) CheckModPubkeyGlobal(pubkey string) bool {
	var result bool
	result, _ = self.client.SIsMember(MOD_KEY_PREFIX+pubkey+"::Group::"+"overchan"+"::Permissions", "all").Result()
	return result
}

func (self RedisDB) CheckModPubkeyCanModGroup(pubkey, newsgroup string) (bool, error) {
	result, err := self.client.SIsMember(MOD_KEY_PREFIX+pubkey+"::Group::"+newsgroup+"::Permissions", "default").Result()
	return result, err
}

func (self RedisDB) CountPostsInGroup(newsgroup string, time_frame int64) (result int64, err error) {
	now := util.TimeNow()
	if time_frame > 0 {
		time_frame = now - time_frame
	} else if time_frame < 0 {
		time_frame = 0
	}
	result, err = self.client.ZCount(GROUP_ARTICLE_POSTTIME_WKR_PREFIX+newsgroup, strconv.Itoa(int(time_frame)), strconv.Itoa(int(now))).Result()
	return
}

func (self RedisDB) CheckModPubkey(pubkey string) (bool, error) {
	result, err := self.client.SIsMember(MOD_KEY_PREFIX+pubkey+"::Group::"+"ctl"+"::Permissions", "login").Result()
	return result, err
}

func (self RedisDB) BanArticle(messageID, reason string) error {
	has, err := self.ArticleBanned(messageID)
	if has {
		log.Println(messageID, "already banned")
		return err
	}
	_, err = self.client.HMSet(BANNED_ARTICLE_PREFIX+messageID, "message_id", messageID, "time_banned", strconv.Itoa(int(util.TimeNow())), "ban_reason", reason).Result()
	return err
}

func (self RedisDB) ArticleBanned(messageID string) (result bool, err error) {
	result, err = self.client.Exists(BANNED_ARTICLE_PREFIX + messageID).Result()
	return
}

func (self RedisDB) GetEncAddress(addr string) (encaddr string, err error) {
	var exists bool
	exists, err = self.client.Exists(ADDRS_ENCRYPTED_ADDRS_PREFIX + addr).Result()
	if err == nil {
		if !exists {
			// needs to be inserted
			var key string
			key, encaddr = util.NewAddrEnc(addr)
			if len(encaddr) == 0 {
				err = errors.New("failed to generate new encryption key")
			} else {
				self.client.HMSet(ENCRYPTED_ADDRS_PREFIX+encaddr, "enckey", key, "encaddr", encaddr, "addr", addr)
				_, err = self.client.Set(ADDRS_ENCRYPTED_ADDRS_PREFIX+addr, encaddr, 0).Result()
			}
		} else {
			encaddr, err = self.client.Get(ADDRS_ENCRYPTED_ADDRS_PREFIX + addr).Result()
		}
	}
	return
}

func (self RedisDB) GetEncKey(encAddr string) (enckey string, err error) {
	enckey, err = self.client.HGet(ENCRYPTED_ADDRS_PREFIX+encAddr, "enckey").Result()
	return
}

func (self RedisDB) CheckIPBanned(addr string) (banned bool, err error) {
	banned, err = self.client.Exists(IP_BAN_PREFIX + addr).Result()
	if banned {
		return
	}
	isnet, ipnet := util.IsSubnet(addr)
	var start string
	var range_start string

	if isnet {
		min, max := util.IPNet2MinMax(ipnet)
		range_start = util.ZeroIPString(min)
		start = util.ZeroIPString(max)
	} else {
		ip := net.ParseIP(addr)
		if ip == nil {
			return false, errors.New("Couldn't parse IP")
		}
		start = util.ZeroIPString(ip)
		range_start = start
	}
	res, err := self.client.ZRangeByLex(IP_RANGE_BAN_KR, redis.ZRangeByScore{Min: "[" + start, Max: "+", Count: 1}).Result()
	if err == nil && len(res) > 0 {
		var range_min string
		range_max := res[0]
		range_min, err = self.client.HGet(IP_RANGE_BAN_PREFIX+range_max, "start").Result()
		if err != nil {
			return
		}
		banned = strings.Compare(range_start, range_min) >= 0
	}

	return
}

func (self RedisDB) GetIPAddress(encaddr string) (addr string, err error) {
	var exists bool
	exists, err = self.client.Exists(ENCRYPTED_ADDRS_PREFIX + encaddr).Result()
	if err == nil && exists {
		addr, err = self.client.HGet(ENCRYPTED_ADDRS_PREFIX+encaddr, "addr").Result()
	}
	return
}

func (self RedisDB) MarkModPubkeyGlobal(pubkey string) (err error) {
	if len(pubkey) != 64 {
		err = errors.New("invalid pubkey length")
		return
	}
	if self.CheckModPubkeyGlobal(pubkey) {
		// already marked
		log.Println("pubkey already marked as global", pubkey)
	} else {
		_, err = self.client.SAdd(MOD_KEY_PREFIX+pubkey+"::Group::"+"overchan"+"::Permissions", "all").Result()
	}
	return
}

func (self RedisDB) UnMarkModPubkeyGlobal(pubkey string) (err error) {
	if self.CheckModPubkeyGlobal(pubkey) {
		// already marked
		_, err = self.client.SRem(MOD_KEY_PREFIX+pubkey+"::Group::"+"overchan"+"::Permissions", "all").Result()
	} else {
		err = errors.New("public key not marked as global")
	}
	return
}

func (self RedisDB) CountThreadReplies(root_message_id string) (repls int64, err error) {
	repls, err = self.client.ZCard(THREAD_POST_WKR + root_message_id).Result()
	return
}

func (self RedisDB) GetRootPostsForExpiration(newsgroup string, threadcount int) (roots []string, err error) {
	roots, err = self.client.ZRange(GROUP_THREAD_POSTTIME_WKR_PREFIX+newsgroup, 0, int64(-threadcount-1)).Result()
	return
}

func (self RedisDB) GetAllNewsgroups() (groups []string, err error) {
	groups, err = self.client.ZRevRange(GROUP_POSTTIME_WKR, 0, -1).Result()
	return
}

func (self RedisDB) GetGroupPageCount(newsgroup string) (int64, error) {
	var count int64
	var err error
	count, err = self.client.ZCard(GROUP_THREAD_POSTTIME_WKR_PREFIX + newsgroup).Result()
	if err != nil {
		return 0, err
	}

	if count > 0 {
		// divide by threads per page
		perpage, _ := self.GetPagesPerBoard(newsgroup)
		pages := int64(math.Floor(float64(count-1)/float64(perpage))) + 1
		return pages, nil
	}
	return 1, nil
}

// only fetches root posts
// does not update the thread contents
func (self RedisDB) GetGroupForPage(newsgroup string, pageno, perpage int) (*model.BoardPage, error) {
	pages, _ := self.GetGroupPageCount(newsgroup)
	threadids, err := self.client.ZRevRange(GROUP_THREAD_BUMPTIME_WKR_PREFIX+newsgroup, int64(pageno*perpage), int64(pageno*perpage+perpage-1)).Result()
	threads := make([]model.Thread, len(threadids))

	if err == nil {
		for i, msgid := range threadids {
			threads[i], _ = self.GetThread(msgid)
		}
	}

	return &model.BoardPage{
		Board:   newsgroup,
		Page:    pageno,
		Pages:   int(pages),
		Threads: threads,
	}, nil
}

/*
func (self RedisDB) GetPostsInGroup(newsgroup string) (models []model.Post, err error) {
	var posts []string
	posts, err = self.client.ZRange(GROUP_ARTICLE_POSTTIME_WKR_PREFIX+newsgroup, 0, -1).Result()
	if err == nil {
		for _, msgid := range posts {
			p, _ := self.GetPost(msgid)
			models = append(models, p)
		}
	}
	return
}*/

func (self RedisDB) GetPost(messageID string) (*model.Post, error) {
	var p model.Post
	cmd := self.client.HGetAll(ARTICLE_POST_PREFIX + messageID)
	hashres, err := cmd.Result()
	if err == nil {
		mapRes := processHashResult(hashres)
		p.Board = mapRes["newsgroup"]
		p.Message_id = mapRes["message_id"]
		p.Parent = mapRes["ref_id"]
		p.PostName = mapRes["name"]
		p.PostSubject = mapRes["subject"]
		p.MessagePath = mapRes["path"]
		tmp, _ := strconv.Atoi(mapRes["time_posted"])
		p.Posted = int64(tmp)
		p.Addr = mapRes["addr"]
		p.PostMessage = mapRes["message"]

		p.OP = len(p.Parent) == 0
		if len(p.Parent) == 0 {
			p.Parent = p.Message_id
		}
		p.Sage = util.IsSage(p.PostSubject)
		atts, err := self.GetPostAttachments(messageID)
		if atts != nil {
			p.Files = append(p.Files, atts...)
		}
		// quiet fail
		p.Key, _ = self.client.Get(ARTICLE_KEY_PREFIX + messageID).Result()
		return &p, err
	}
	return &p, err
}

func (self RedisDB) DeleteThread(msgid string) error {
	repls, err := self.GetThreadReplies(msgid, 0, 0)
	for _, r := range repls {
		self.DeleteArticle(r)
	}
	group, _ := self.client.HGet(ARTICLE_PREFIX+msgid, "message_newsgroup").Result()
	if group != "" {
		self.client.ZRem(GROUP_THREAD_POSTTIME_WKR_PREFIX+group, msgid)
		self.client.ZRem(GROUP_THREAD_BUMPTIME_WKR_PREFIX+group, msgid)
	}
	self.client.ZRem(THREAD_BUMPTIME_WKR, msgid)
	self.client.Del(THREAD_POST_WKR + msgid)
	self.DeleteArticle(msgid)

	return err
}

func (self RedisDB) ForgetArticle(msgid string) (err error) {
	_, err = self.client.Del(ARTICLE_PREFIX+msgid, ARTICLE_POST_PREFIX+msgid, ARTICLE_KEY_PREFIX+msgid).Result()
	return
}

func (self RedisDB) DeleteArticle(msgid string) error {
	p, err := self.GetPost(msgid)
	if err == nil {
		if !p.OP {
			self.client.ZRem(THREAD_POST_WKR+p.Parent, msgid)
		}
		hash, _ := self.client.HGet(ARTICLE_PREFIX+msgid, "message_id_hash").Result()
		if hash != "" {
			self.client.Del(HASH_MESSAGEID_PREFIX + hash)
		}

		//self.client.Del(ARTICLE_PREFIX+msgid, ARTICLE_POST_PREFIX+msgid, ARTICLE_KEY_PREFIX+msgid)
		self.client.ZRem(GROUP_ARTICLE_POSTTIME_WKR_PREFIX+p.Board, msgid)
		self.client.ZRem(ARTICLE_WKR, msgid)

		headers, _ := self.client.SMembers(MESSAGEID_HEADER_KR_PREFIX + msgid).Result()
		for _, h := range headers {
			self.client.SRem(HEADER_KR_PREFIX+h, msgid)
		}
		self.client.Del(MESSAGEID_HEADER_KR_PREFIX + msgid)

		atts, _ := self.client.SMembers(ARTICLE_ATTACHMENT_KR_PREFIX + msgid).Result()
		for _, a := range atts {
			self.client.SRem(ATTACHMENT_ARTICLE_KR_PREFIX+a, msgid)
			exists, _ := self.client.Exists(ATTACHMENT_ARTICLE_KR_PREFIX + a).Result()
			if !exists { //no other post uses this attachment any more
				//TODO delete files from disk
				self.client.Del(ATTACHMENT_PREFIX + a)
			}
		}
		self.client.Del(ARTICLE_ATTACHMENT_KR_PREFIX + msgid)
		self.client.ZRem(ARTICLE_NUMBERS_PREFIX+"group::"+p.Board, msgid)

		addr := p.Addr
		ip := net.ParseIP(addr)
		if ip != nil {
			addr = util.ZeroIPString(ip)
		}
		self.client.SRem(IP_ARTICLE_KR_PREFIX+addr, msgid)
		if ip != nil {
			count, err := self.client.SRem(IP_ARTICLE_KR_PREFIX + addr).Result()
			if err == nil && count <= 0 {
				self.client.ZRem(IP_WKR, addr)
			}
		}
	}
	return err
}

func (self RedisDB) GetThread(rootpost string) (model.Thread, error) {
	posts, err := self.GetThreadReplies(rootpost, 0, 0)
	repls := make([]*model.Post, len(posts))

	for i, msgid := range posts {
		repls[i], _ = self.GetPost(msgid)
	}
	root, _ := self.GetPost(rootpost)

	return model.Thread{root, repls}, err

}

func (self RedisDB) GetThreadReplies(rootpost string, start, limit int) (repls []string, err error) {
	if limit < 1 {
		limit = 1
	}
	repls, err = self.client.ZRange(THREAD_POST_WKR+rootpost, int64((start-limit)+1), -1).Result()
	return
}

func (self RedisDB) ThreadHasReplies(rootpost string) (bool, error) {
	count, err := self.client.ZCard(THREAD_POST_WKR + rootpost).Result()
	return count > 0, err
}

func (self RedisDB) GetGroupThreads(group string, recv chan model.ArticleEntry) error {
	threads, err := self.client.ZRange(GROUP_THREAD_BUMPTIME_WKR_PREFIX+group, 0, -1).Result()
	if err == nil {
		for _, msgid := range threads {
			recv <- model.ArticleEntry{msgid, group}
		}
	}
	return err
}

func (self RedisDB) GetUkkoThreads(max int) ([]*model.Thread, error) {
	threads, err := self.client.ZRevRange(THREAD_BUMPTIME_WKR, 0, int64(max-1)).Result()
	roots := make([]*model.Thread, len(threads))
	if err == nil {
		for i, msgid := range threads {
			t, _ := self.GetThread(msgid)
			roots[i] = &t
		}
	}
	return roots, err
}

func (self RedisDB) GroupHasPosts(group string) (bool, error) {
	count, err := self.client.ZCard(GROUP_THREAD_BUMPTIME_WKR_PREFIX + group).Result()
	return count > 0, err
}

// check if a newsgroup exists
func (self RedisDB) HasNewsgroup(group string) (bool, error) {
	_, err := self.client.ZRank(GROUP_POSTTIME_WKR, group).Result()
	return err == nil, nil
}

// check if an article exists
func (self RedisDB) SeenArticle(message_id string) (bool, error) {
	res, err := self.client.Exists(ARTICLE_PREFIX + message_id).Result()
	return res, err
}

// check if an article exists locally
func (self RedisDB) HasArticle(message_id string) (bool, error) {
	res, err := self.client.Exists(ARTICLE_POST_PREFIX + message_id).Result()
	return res, err
}

// count articles we have
func (self RedisDB) ArticleCount() (count int64, err error) {
	count, err = self.client.ZCard(ARTICLE_WKR).Result()
	return
}

// register a new newsgroup
func (self RedisDB) RegisterNewsgroup(group string) error {
	_, err := self.client.ZAddNX(GROUP_POSTTIME_WKR, redis.Z{Score: float64(util.TimeNow()), Member: group}).Result()
	return err
}

func (self RedisDB) GetPostAttachments(messageID string) ([]*model.Attachment, error) {
	hashes, err := self.client.SMembers(ARTICLE_ATTACHMENT_KR_PREFIX + messageID).Result()
	atts := make([]*model.Attachment, len(hashes))
	if err == nil {
		for i, hash := range hashes {

			hashres, _ := self.client.HGetAll(ATTACHMENT_PREFIX + hash).Result()
			mapRes := processHashResult(hashres)

			atts[i] = &model.Attachment{mapRes["filepath"], mapRes["filename"], mapRes["mime"], mapRes["sha_hash"]}
		}
	}
	return atts, err
}

// register a message with the database
func (self RedisDB) RegisterArticle(message *model.Article) (err error) {
	pipe := self.client.Pipeline()
	defer pipe.Close()

	msgid := message.MessageID
	group := message.Newsgroup

	has, _ := self.HasNewsgroup(group)
	if !has {
		self.RegisterNewsgroup(group)
	}
	has, _ = self.HasArticle(msgid)
	if has {
		return
	}

	//insert IP

	addr := message.Addr
	ip := net.ParseIP(addr)

	if ip != nil {
		addr = util.ZeroIPString(ip)
		pipe.ZAddNX(IP_RANGE_BAN_KR, redis.Z{Score: 0.0, Member: addr})
	}
	pipe.SAdd(IP_ARTICLE_KR_PREFIX+addr, msgid)

	now := util.TimeNow()

	// insert article metadata
	pipe.HMSet(ARTICLE_PREFIX+msgid, "msgid", msgid, "message_id_hash", util.HashMessageID(msgid), "message_newsgroup", group, "time_obtained", strconv.Itoa(int(now)), "message_ref_id", message.Reference)
	pipe.Set(HASH_MESSAGEID_PREFIX+util.HashMessageID(msgid), msgid, 0)

	// update newsgroup
	pipe.ZAddXX(GROUP_POSTTIME_WKR, redis.Z{Score: float64(now), Member: group})
	pipe.ZAddNX(GROUP_ARTICLE_POSTTIME_WKR_PREFIX+group, redis.Z{Score: float64(now), Member: msgid})

	// insert article post
	pipe.HMSet(ARTICLE_POST_PREFIX+msgid, "newsgroup", group, "message_id", msgid, "ref_id", message.Reference, "name", message.Name, "subject", message.Subject, "path", message.Path, "time_posted", strconv.Itoa(int(message.Posted)), "message", message.Text, "addr", addr)

	if group != "ctl" { // control messages aren't added to the global keyring
		pipe.ZAddNX(ARTICLE_WKR, redis.Z{Score: float64(now), Member: msgid})
	}

	// set / update thread state
	if message.Reference == "" {
		// insert new thread for op
		pipe.ZAddNX(GROUP_THREAD_POSTTIME_WKR_PREFIX+group, redis.Z{Score: float64(message.Posted), Member: msgid})
		pipe.ZAddNX(GROUP_THREAD_BUMPTIME_WKR_PREFIX+group, redis.Z{Score: float64(message.Posted), Member: msgid})
		if group != "ctl" {
			pipe.ZAddNX(THREAD_BUMPTIME_WKR, redis.Z{Score: float64(message.Posted), Member: msgid})
		}

	} else {
		ref := message.Reference
		if !util.IsSage(message.Subject) {
			// bump it nigguh
			pipe.ZAddXX(GROUP_THREAD_BUMPTIME_WKR_PREFIX+group, redis.Z{Score: float64(message.Posted), Member: ref})
			pipe.ZAddXX(THREAD_BUMPTIME_WKR, redis.Z{Score: float64(message.Posted), Member: ref})
		}
		// update last posted
		pipe.ZAddXX(GROUP_THREAD_POSTTIME_WKR_PREFIX+group, redis.Z{Score: float64(message.Posted), Member: ref})
		pipe.ZAddNX(THREAD_POST_WKR+ref, redis.Z{Score: float64(message.Posted), Member: msgid})
	}
	// register article header
	for k, val := range message.Header {
		k = strings.ToLower(k)
		for _, v := range val {
			k = strings.ToLower(k)
			header := "Name::" + k + "::Value::" + v
			pipe.SAdd(HEADER_KR_PREFIX+header, msgid)
			pipe.SAdd(MESSAGEID_HEADER_KR_PREFIX+msgid, header)
		}
	}
	// add nntp message numbers
	number, _ := self.client.ZIncrBy(ARTICLE_NUMBERS_PREFIX+"last", float64(1), group).Result()
	pipe.ZAddNX(ARTICLE_NUMBERS_PREFIX+"group::"+group, redis.Z{Score: number, Member: msgid})

	// register all attachments
	atts := message.Attachments
	if atts != nil {
		for _, att := range atts {
			hash := att.Hash
			pipe.SAdd(ATTACHMENT_ARTICLE_KR_PREFIX+hash, msgid)
			pipe.SAdd(ARTICLE_ATTACHMENT_KR_PREFIX+msgid, hash)
			pipe.HSetNX(ATTACHMENT_PREFIX+hash, "message_id", msgid)
			pipe.HSetNX(ATTACHMENT_PREFIX+hash, "sha_hash", hash)
			pipe.HSetNX(ATTACHMENT_PREFIX+hash, "filename", att.Name)
			pipe.HSetNX(ATTACHMENT_PREFIX+hash, "filepath", att.Path)
			pipe.HSetNX(ATTACHMENT_PREFIX+hash, "mime", att.Mime)
		}
	}

	_, err = pipe.Exec()
	if err != nil {
		log.Println("failed to register nntp article", err)
	}
	return
}

//
// get message ids of articles with this header name and value
//
func (self RedisDB) GetMessageIDByHeader(name, val string) (msgids []string, err error) {
	header := "Name::" + name + "::Value::" + val
	msgids, err = self.client.SMembers(HEADER_KR_PREFIX + header).Result()
	return
}

func (self RedisDB) RegisterSigned(message_id, pubkey string) (err error) {
	_, err = self.client.Set(ARTICLE_KEY_PREFIX+message_id, pubkey, 0).Result()
	return
}

// get all articles in a newsgroup
// send result down a channel
func (self RedisDB) GetAllArticlesInGroup(group string, recv chan model.ArticleEntry) error {
	articles, err := self.client.ZRange(GROUP_ARTICLE_POSTTIME_WKR_PREFIX+group, 0, -1).Result()
	if err == nil {
		for _, msgid := range articles {
			recv <- model.ArticleEntry{msgid, group}
		}
	}
	return err
}

// get all articles
func (self RedisDB) GetAllArticles() (articles []model.ArticleEntry, err error) {
	var articleids []string
	articleids, err = self.client.ZRange(ARTICLE_WKR, 0, -1).Result()
	if err == nil {
		for _, msgid := range articleids {
			group, _ := self.GetGroupForMessage(msgid) //this seems expensive. it might be a better idea to add the group to ARTICLE_WKR
			articles = append(articles, model.ArticleEntry{msgid, group})
		}
	}
	return
}

func (self RedisDB) GetPagesPerBoard(group string) (int, error) {
	//XXX: hardcoded
	return 10, nil
}

func (self RedisDB) GetThreadsPerPage(group string) (int, error) {
	//XXX: hardcoded
	return 10, nil
}

func (self RedisDB) GetMessageIDByCIDR(cidr *net.IPNet) (msgids []string, err error) {
	min, max := util.IPNet2MinMax(cidr)
	start := util.ZeroIPString(min)
	stop := util.ZeroIPString(max)
	res, err := self.client.ZRangeByLex(IP_WKR, redis.ZRangeByScore{Min: "[" + start, Max: "[" + stop}).Result()
	if err == nil && len(res) > 0 {
		for _, ip := range res {
			posts, _ := self.client.SMembers(IP_ARTICLE_KR_PREFIX + ip).Result()
			msgids = append(msgids, posts...)
		}
	}
	return
}

func (self RedisDB) GetMessageIDByEncryptedIP(encip string) (msgids []string, err error) {
	msgids, err = self.client.SMembers(IP_ARTICLE_KR_PREFIX + encip).Result()
	return
}

func (self RedisDB) GetMessageIDByHash(hash string) (article model.ArticleEntry, err error) {
	var msgid string
	var group string
	msgid, err = self.client.Get(HASH_MESSAGEID_PREFIX + hash).Result()
	if err == nil {
		group, err = self.GetGroupForMessage(msgid)
		if err == nil {
			article = model.ArticleEntry{msgid, group}
		}
	}
	return
}

func (self RedisDB) BanAddr(addr string) (err error) {
	isnet, ipnet := util.IsSubnet(addr)
	if !isnet {
		_, err = self.client.HMSet(IP_BAN_PREFIX+addr, "addr", addr, "made", strconv.Itoa(int(util.TimeNow()))).Result()
		return
	}
	isBanned, err := self.CheckIPBanned(addr)
	if !isBanned && err == nil { //make sure this range isn't banned already
		min, max := util.IPNet2MinMax(ipnet)
		start := util.ZeroIPString(min)
		end := util.ZeroIPString(max)
		self.clearIPRange(start, end) //delete all banned ranges that are contained within this range
		_, err = self.client.ZAdd(IP_RANGE_BAN_KR, redis.Z{Score: 0.0, Member: end}).Result()

		if err != nil {
			return
		}
		_, err = self.client.HMSet(IP_RANGE_BAN_PREFIX+end, "start", start, "end", end, "made", strconv.Itoa(int(util.TimeNow()))).Result()
	}

	return
}

func (self RedisDB) UnbanAddr(addr string) (err error) {
	_, err = self.client.Del(IP_BAN_PREFIX + addr).Result()
	isnet, ipnet := util.IsSubnet(addr)
	var start string
	var range_start string

	if isnet {
		min, max := util.IPNet2MinMax(ipnet)
		range_start = util.ZeroIPString(min)
		start = util.ZeroIPString(max)
	} else {
		_, err = self.client.Del(IP_BAN_PREFIX + addr).Result()
		return
	}
	res, err := self.client.ZRangeByLex(IP_RANGE_BAN_KR, redis.ZRangeByScore{Min: "[" + start, Max: "+", Count: 1}).Result()
	if err == nil && len(res) > 0 {
		var range_min string
		range_max := res[0]
		range_min, err = self.client.HGet(IP_RANGE_BAN_PREFIX+range_max, "start").Result()
		if err != nil {
			return
		}
		banned := strings.Compare(range_start, range_min) >= 0
		if banned {
			self.client.ZRem(IP_RANGE_BAN_KR, range_max)
			self.client.Del(IP_RANGE_BAN_PREFIX + range_max)
		}
	}
	return
}

func (self RedisDB) CheckEncIPBanned(encaddr string) (banned bool, err error) {
	banned, err = self.client.Exists(ENCRYPTED_IP_BAN_PREFIX + encaddr).Result()
	return
}

func (self RedisDB) BanEncAddr(encaddr string) (err error) {
	_, err = self.client.HMSet(ENCRYPTED_IP_BAN_PREFIX+encaddr, "encaddr", encaddr, "made", strconv.Itoa(int(util.TimeNow()))).Result()
	return
}

func (self RedisDB) GetLastAndFirstForGroup(group string) (last, first int64, err error) {
	var minres, maxres []redis.Z
	minres, err = self.client.ZRangeWithScores(ARTICLE_NUMBERS_PREFIX+"group::"+group, 0, 0).Result()
	if err != nil {
		return
	}
	maxres, err = self.client.ZRangeWithScores(ARTICLE_NUMBERS_PREFIX+"group::"+group, -1, -1).Result()
	if err != nil || len(minres) < 1 || len(maxres) < 1 {
		return
	}
	first = int64(minres[0].Score)
	last = int64(maxres[0].Score)

	return
}

func (self RedisDB) GetMessageIDForNNTPID(group string, id int64) (msgid string, err error) {
	var msgs []string

	// widen the range a bit just to be sure with float precision
	min := float64(id) - 0.0001
	max := float64(id) + 0.0001
	mins := fmt.Sprintf("%f", min)
	maxs := fmt.Sprintf("%f", max)

	msgs, err = self.client.ZRangeByScore(ARTICLE_NUMBERS_PREFIX+"group::"+group, redis.ZRangeByScore{Min: mins, Max: maxs}).Result()
	if err == nil && len(msgs) > 0 {
		msgid = msgs[0]
	}
	return
}

func (self RedisDB) GetNNTPIDForMessageID(group, msgid string) (id int64, err error) {
	var res float64
	res, err = self.client.ZScore(ARTICLE_NUMBERS_PREFIX+"group::"+group, msgid).Result()
	id = int64(res)
	return
}

func (self RedisDB) MarkModPubkeyCanModGroup(pubkey, group string) (err error) {
	_, err = self.client.SAdd(MOD_KEY_PREFIX+pubkey+"::Group::"+group+"::Permissions", "default").Result()
	self.client.SAdd(GROUP_MOD_KEY_REVERSE_KR_PREFIX+group, pubkey)
	return
}

func (self RedisDB) UnMarkModPubkeyCanModGroup(pubkey, group string) (err error) {
	_, err = self.client.SRem(MOD_KEY_PREFIX+pubkey+"::Group::"+group+"::Permissions", "default").Result()
	self.client.SRem(GROUP_MOD_KEY_REVERSE_KR_PREFIX+group, pubkey)
	return
}

func (self RedisDB) IsExpired(root_message_id string) (bool, error) {
	has1, err1 := self.SeenArticle(root_message_id)
	has2, err2 := self.HasArticle(root_message_id)

	if err1 != nil {
		return false, err1
	}

	return has1 && !has2, err2
}

func (self RedisDB) GetLastDaysPostsForGroup(newsgroup string, n int64) (posts []model.PostEntry, err error) {
	day := time.Hour * 24
	now := time.Now().UTC()
	now = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	for n > 0 {
		min := strconv.Itoa(int(now.Unix()))
		max := strconv.Itoa(int(now.Add(day).Unix()))
		num, err := self.client.ZCount(GROUP_ARTICLE_POSTTIME_WKR_PREFIX+newsgroup, min, max).Result()
		if err == nil {
			posts = append(posts, model.PostEntry{now.Unix(), num})
			now = now.Add(-day)
		} else {
			return nil, err
		}
		n--
	}
	return
}

func (self RedisDB) GetLastDaysPosts(n int64) (posts []model.PostEntry, err error) {
	day := time.Hour * 24
	now := time.Now().UTC()
	now = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	for n > 0 {
		min := strconv.Itoa(int(now.Unix()))
		max := strconv.Itoa(int(now.Add(day).Unix()))
		num, err := self.client.ZCount(ARTICLE_WKR, min, max).Result()
		if err == nil {
			posts = append(posts, model.PostEntry{now.Unix(), num})
			now = now.Add(-day)
		} else {
			return nil, err
		}
		n--
	}
	return posts, nil
}

/*
func (self RedisDB) GetLastPosted(n int64) (posts []model.Post) {
	messages, err := self.client.ZRevRange(ARTICLE_WKR, 0, n-1).Result()
	if err == nil {
		for _, msgid := range messages {
			model := self.GetPost(msgid)
			posts = append(posts, model)
		}
		return
	} else {
		log.Println("failed to prepare query for geting last post models", err)
		return nil
	}
}*/

func (self RedisDB) GetMonthlyPostHistory() (posts []model.PostEntry, err error) {
	var oldest int64
	now := time.Now()
	now = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	res, err := self.client.ZRangeWithScores(ARTICLE_WKR, 0, 0).Result()
	if err == nil && len(res) > 0 {
		// we got the oldest
		oldest = int64(res[0].Score)
		// convert it to the oldest year/date
		old := time.Unix(oldest, 0)
		old = time.Date(old.Year(), old.Month(), 1, 0, 0, 0, 0, time.UTC)
		// count up from oldest to newest
		for now.Unix() >= old.Unix() {
			var next_month time.Time
			if now.Month() < 12 {
				next_month = time.Date(old.Year(), old.Month()+1, 1, 0, 0, 0, 0, time.UTC)
			} else {
				next_month = time.Date(old.Year()+1, 1, 1, 0, 0, 0, 0, time.UTC)
			}
			// get the post count in that montth
			min := strconv.Itoa(int(old.Unix()))
			max := strconv.Itoa(int(next_month.Unix()))
			count, err := self.client.ZCount(ARTICLE_WKR, min, max).Result()
			if err == nil {
				posts = append(posts, model.PostEntry{old.Unix(), count})
				old = next_month
			} else {
				posts = nil
				break
			}
		}
	}
	return
}

func (self RedisDB) CheckNNTPLogin(username, passwd string) (valid bool, err error) {
	var login_hash, login_salt string
	var hashres []string
	hashres, err = self.client.HGetAll(NNTP_LOGIN_PREFIX + username).Result()

	if err == nil {
		// no errors
		mapRes := processHashResult(hashres)

		login_hash = mapRes["login_hash"]
		login_salt = mapRes["login_salt"]

		if len(login_hash) > 0 && len(login_salt) > 0 {
			valid = util.NntpLoginCredHash(passwd, login_salt) == login_hash
		}
	}
	return
}

func (self RedisDB) AddNNTPLogin(username, passwd string) (err error) {
	login_salt := util.GenLoginCredSalt()
	login_hash := util.NntpLoginCredHash(passwd, login_salt)
	_, err = self.client.HMSet(NNTP_LOGIN_PREFIX+username, "username", username, "login_hash", login_hash, "login_salt", login_salt).Result()
	return
}

func (self RedisDB) RemoveNNTPLogin(username string) (err error) {
	_, err = self.client.Del(NNTP_LOGIN_PREFIX + username).Result()
	return
}

func (self RedisDB) CheckNNTPUserExists(username string) (exists bool, err error) {
	exists, err = self.client.Exists(NNTP_LOGIN_PREFIX + username).Result()
	return
}

func (self RedisDB) clearIPRange(start, end string) {
	ranges, _ := self.client.ZRangeByLex(IP_RANGE_BAN_KR, redis.ZRangeByScore{Min: "(" + start, Max: "[" + end}).Result()
	for _, iprange := range ranges {
		self.client.ZRem(IP_RANGE_BAN_KR, iprange)
		self.client.Del(IP_RANGE_BAN_PREFIX + iprange)
	}
}

func (self RedisDB) GetHeadersForMessage(msgid string) (hdr model.ArticleHeader, err error) {
	var members []string
	members, err = self.client.SMembers(MESSAGEID_HEADER_KR_PREFIX + msgid).Result()
	if err == nil {
		hdr = make(model.ArticleHeader)
		for _, member := range members {
			k := member[6:]
			idx := strings.Index(k, "::Value::")
			if idx < 0 {
				continue
			}
			v := k[idx+9:]
			k = k[:idx]
			hdr[k] = append(hdr[k], v)
		}
	}
	return
}

func (self RedisDB) CountAllArticlesInGroup(group string) (count int64, err error) {
	count, err = self.client.ZCard(GROUP_ARTICLE_POSTTIME_WKR_PREFIX + group).Result()
	return
}

func processHashResult(hash []string) (mapRes map[string]string) {
	mapRes = make(map[string]string)
	max := len(hash)
	for i := 0; i < max; i += 2 {
		mapRes[hash[i]] = hash[i+1]
	}
	return
}

func RedisEnabled() bool {
	return true
}
