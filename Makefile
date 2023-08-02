build:
	go build -o bin/weather-bot main.go

run: build
	./bin/weather-bot

test:
	go test -v ./... -count=1

#docker-up:
#	docker-compose -f docker-compose.debug.yml up --build
