```Bash
# 编译
go build -o bin/upstream ./cmd/upstream

go build -o bin/gatesvr ./cmd/gatesvr

go build -o bin/client ./cmd/client

go build -o bin/monitor.exe ./cmd/monitor

go build -o bin/gencerts ./cmd/gencerts

# 运行

./bin/upstream -addr ":8081"
./bin/upstream -addr ":8082"
./bin/upstream -addr ":8083"

./bin/gatesvr -config test-config.yaml

./bin/client -server localhost:8453 -interactive
./bin/client -server localhost:8453 -performance -clients 1500 -openid-start 14100 -openid-end 16000 -interval 200ms

./bin/monitor -server localhost:8080 -continuous -interval 2s
```
