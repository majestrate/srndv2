# SRNDv2 #

Some Random News Daemon version 2

overchan nntp daemon

status: sorta working

donate: bitcoin 15yuMzuueV8y5vPQQ39ZqQVz5Ey98DNrjE
	

## requirements ##

* a modern c compiler, (gcc, clang)
* libsodium
* go 1.4 or higher
* postgresql

## building

    # get dependancies
    go get github.com/gorilla/sessions
    go get github.com/gorilla/mux
    go get github.com/dchest/captcha
    go get github.com/majestrate/configparser
    go get github.com/lib/pq
    go get github.com/hoisie/mustache

    # get libsodium
    sudo apt-get install libsodium-dev
    
    # set gopath if it's not already set
    export GOPATH=$HOME/go
    mkdir -p $GOPATH

    # get source code
    go get github.com/majestrate/srndv2
    cd $GOPATH/src/github.com/majestrate/srndv2

    # build it
    make

## initial run

    # this will generate base config files if they aren't present
    ./srnd setup

## database configuration

In order to use srndv2 you need to have a working postgres database that you can access.

If you haven't already, install postgresql

    sudo apt-get install postgresql postgresql-client

Create a new Role and Database for srndv2, on ubuntu/debian you'll need to get a psql shell as the postgres user

    su postgres
    psql

then create the database credentials, make sure to use your own password :^)

    CREATE ROLE srnduser WITH LOGIN PASSWORD 'srndpassword';
    CREATE DATABASE srnd WITH ENCODING 'UTF8' OWNER srnduser;
    \q

then edit the database section in srnd.ini so it has the proper parameters

    [database]
    type=postgres
    schema=srnd
    host=127.0.0.1
    port=5432
    user=srnduser
    password=srndpassword

## running

    # after you have configured the daemon, run it
    # by default, an http daemon will bind on all interfaces port 18000
    # if your server is 1.2.34.5 access it via your web browser at http://1.2.34.5:18000/ 
    ./srnd run

## running on tor

**IMPORTANT** you *must* bind your web interface to loopback if you use tor, if you don't know how, rtfm plz.

## additional configuration

    see config.md
