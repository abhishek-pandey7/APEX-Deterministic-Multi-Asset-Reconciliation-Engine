.PHONY: test race vet check

test:
	go test ./...

race:
	go test -race ./...

vet:
	go vet ./...

check: vet race
