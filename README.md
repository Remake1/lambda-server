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

### API Routes

| Type | Path           |
|------|----------------|
| POST | /auth/register |
| POST | /auth/login    |
| POST | /auth/refresh  |

### WebSocket Protocol

| Flow    | Sender   | Receiver  | Type                    | Example Payload (JSON)              |                                                       |
|---------|----------|-----------|-------------------------|-------------------------------------|-------------------------------------------------------|
| C2S     | Client   | Server    | websocket.TextMessage   | {""type"": ""command_to_hardware"   | "payload"": {""command"": ""get_image""}}             |
| S2H     | Server   | Hardware  | websocket.TextMessage   | {""type"": ""command_to_hardware"   | "payload"": {""command"": ""get_image""}} (Forwarded) |
| H2S     | Hardware | Server    | websocket.BinaryMessage | [...raw image bytes...]             |                                                       |
| H2S     | Hardware | Server    | websocket.TextMessage   | {""type"": ""status"                | "payload"": ""camera_initializing""} (Optional)       |
| S2C     | Server   | Client    | websocket.TextMessage   | {""type"": ""image_analysis_result" | "payload"": {""image_size"": 123456}}                 |
| S2(C/H) | Server   | Client/HW | websocket.TextMessage   | {""type"": ""system_status"         | "payload"": {""message"": ""hardware_connected""}}    |