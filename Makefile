build:
	docker build -t omarkhd/hraftd:latest .

down:
	docker-compose down --remove-orphans

up: down build
	docker-compose up
