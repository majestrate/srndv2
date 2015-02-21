# SRNDv2 #

Some Random News Daemon version 2

overchan nntp daemon

status: in dev

donate: bitcoin 15yuMzuueV8y5vPQQ39ZqQVz5Ey98DNrjE
	

## requirements ##

* go 1.4 or higher

## dependancies

    go get github.com/majestrate/configparser
    go get github.com/lib/pq

## Notes 

When receiving many articles (i.e. durring initial sync with a network with over
9000 posts (or so) disable outfeeds as there is an unresolved race condition
