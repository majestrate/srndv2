

## TODO LIST ##

* OAUTH API
* mysql database type
* sqlite database type
* redis database type


--------

src/srnd/config.go 

Setup() method needs to catch errors.
ReadConfig() : Should take config file name as argument. Should return error type if erorr occurs.

func ReadConfig(filename string) (*SRNdConfig, error){}
