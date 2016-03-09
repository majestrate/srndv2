// +build disable_redis

package srnd

func NewRedisDatabase(host, port, password string) Database {
	panic("Redis was disabled at compile time!");

	return NewPostgresDatabase("", "", "", ""); //this shouldn't be reached
}

func NewRedisCache(prefix, webroot, name string, threads int, attachments bool, db Database, host, port, password string) CacheInterface {
	panic("Redis was disabled at compile time!");
	
	return NewFileCache(prefix, webroot, name, threads, attachments, db, nil) //this shouldn't be reached
}
