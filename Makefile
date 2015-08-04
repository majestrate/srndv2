all: clean build

build:
	go build -o srnd srnd.go
clean:
	rm -f srnd
