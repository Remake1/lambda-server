# Lambda server

### Requirements
- Go 1.25
- PostgreSQL/Docker
- Google Cloud API Key

### Setup with docker

1. Install docker
2. Setup environment variables in `.env` file (JWT_SECRET_KEY and GOOGLE_API_KEY)
3. Run `docker-compose build`
4. Run `docker-compose up -d`

### Run for development

Set up database:
```shell
docker-compose up -d db                                                                        
```
Start server:
```shell
go run ./cmd/server 
```

### Documentation

#### Swagger UI URL: `http://localhost:3000/swagger/index.html`

#### Generate documentation:
```shell
swag init -g ./cmd/server/main.go -o ./docs
```

### HTTP Routes

| Type  | Path           |
|-------|----------------|
| POST  | /auth/register |
| POST  | /auth/login    |
| POST  | /auth/refresh  |
| GET   | /auth/me       |
