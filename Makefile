all:
	go build -o SRNd srnd/*.go
clean:
	rm -f SRNd
