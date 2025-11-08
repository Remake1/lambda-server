### Start server

```shell
docker-compose up -d db                                                                        
```

```shell
go run ./cmd/server 
```

### Protocol

| Flow    | Sender   | Receiver  | Type                    | Example Payload (JSON)              |                                                       |
|---------|----------|-----------|-------------------------|-------------------------------------|-------------------------------------------------------|
| C2S     | Client   | Server    | websocket.TextMessage   | {""type"": ""command_to_hardware"   | "payload"": {""command"": ""get_image""}}             |
| S2H     | Server   | Hardware  | websocket.TextMessage   | {""type"": ""command_to_hardware"   | "payload"": {""command"": ""get_image""}} (Forwarded) |
| H2S     | Hardware | Server    | websocket.BinaryMessage | [...raw image bytes...]             |                                                       |
| H2S     | Hardware | Server    | websocket.TextMessage   | {""type"": ""status"                | "payload"": ""camera_initializing""} (Optional)       |
| S2C     | Server   | Client    | websocket.TextMessage   | {""type"": ""image_analysis_result" | "payload"": {""image_size"": 123456}}                 |
| S2(C/H) | Server   | Client/HW | websocket.TextMessage   | {""type"": ""system_status"         | "payload"": {""message"": ""hardware_connected""}}    |