build:
	docker build -t otoolep/hraftd:latest .

down:
	docker-compose down --remove-orphans

up: down build
	docker-compose up
