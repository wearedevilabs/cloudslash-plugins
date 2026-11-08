BINARY = cloudslash-plugin-cloudslash-plugin-postgres

.PHONY: build test install clean

build:
	go build -o $(BINARY) .

test:
	go test -v ./...

install: build
	mkdir -p ~/.cloudslash/plugins/cloudslash-plugin-postgres
	cp $(BINARY) plugin.json ~/.cloudslash/plugins/cloudslash-plugin-postgres/

clean:
	rm -f $(BINARY)
