# Details

Date : 2025-08-08 19:53:13

Directory d:\\25cxx\\gatesvr

Total : 85 files,  30168 codes, 2197 comments, 3549 blanks, all 35914 lines

[Summary](results.md) / Details / [Diff Summary](diff.md) / [Diff Details](diff-details.md)

## Files
| filename | language | code | comment | blank | total |
| :--- | :--- | ---: | ---: | ---: | ---: |
| [.claude/settings.local.json](/.claude/settings.local.json) | JSON | 10 | 0 | 0 | 10 |
| [.idea/modules.xml](/.idea/modules.xml) | XML | 8 | 0 | 0 | 8 |
| [.idea/v1.iml](/.idea/v1.iml) | XML | 9 | 0 | 0 | 9 |
| [ASYNC\_QUEUE\_IMPLEMENTATION.md](/ASYNC_QUEUE_IMPLEMENTATION.md) | Markdown | 183 | 0 | 42 | 225 |
| [DOCKER-README.md](/DOCKER-README.md) | Markdown | 242 | 0 | 94 | 336 |
| [GRPC\_UNICAST\_SUMMARY.md](/GRPC_UNICAST_SUMMARY.md) | Markdown | 116 | 0 | 38 | 154 |
| [LATENCY\_MONITORING.md](/LATENCY_MONITORING.md) | Markdown | 160 | 0 | 46 | 206 |
| [Makefile](/Makefile) | Makefile | 201 | 35 | 35 | 271 |
| [OPTIMIZATION\_SUMMARY.md](/OPTIMIZATION_SUMMARY.md) | Markdown | 309 | 0 | 80 | 389 |
| [PPROF\_USAGE.md](/PPROF_USAGE.md) | Markdown | 164 | 0 | 46 | 210 |
| [QUICKSTART.md](/QUICKSTART.md) | Markdown | 272 | 0 | 99 | 371 |
| [README-backup.md](/README-backup.md) | Markdown | 158 | 0 | 46 | 204 |
| [SUPPORTED\_ACTIONS.md](/SUPPORTED_ACTIONS.md) | Markdown | 256 | 0 | 58 | 314 |
| [build-docker.bat](/build-docker.bat) | Batch | 79 | 13 | 20 | 112 |
| [build-docker.sh](/build-docker.sh) | Shell Script | 83 | 15 | 22 | 120 |
| [build.bat](/build.bat) | Batch | 45 | 0 | 8 | 53 |
| [cmd/client/main.go](/cmd/client/main.go) | Go | 716 | 68 | 126 | 910 |
| [cmd/gatesvr/main.go](/cmd/gatesvr/main.go) | Go | 367 | 51 | 58 | 476 |
| [cmd/gencerts/main.go](/cmd/gencerts/main.go) | Go | 103 | 8 | 19 | 130 |
| [cmd/monitor/main.go](/cmd/monitor/main.go) | Go | 375 | 11 | 69 | 455 |
| [cmd/upstream/main.go](/cmd/upstream/main.go) | Go | 81 | 11 | 20 | 112 |
| [config-backup-remote.yaml](/config-backup-remote.yaml) | YAML | 25 | 8 | 6 | 39 |
| [config-backup.yaml](/config-backup.yaml) | YAML | 25 | 7 | 5 | 37 |
| [config-primary-remote.yaml](/config-primary-remote.yaml) | YAML | 25 | 8 | 6 | 39 |
| [config.example.yaml](/config.example.yaml) | YAML | 46 | 6 | 8 | 60 |
| [config.yaml](/config.yaml) | YAML | 35 | 11 | 9 | 55 |
| [deploy-docker.bat](/deploy-docker.bat) | Batch | 173 | 17 | 36 | 226 |
| [deploy-docker.sh](/deploy-docker.sh) | Shell Script | 181 | 17 | 31 | 229 |
| [docker-compose.yml](/docker-compose.yml) | YAML | 192 | 7 | 6 | 205 |
| [docker/config/docker-config.yaml](/docker/config/docker-config.yaml) | YAML | 31 | 4 | 4 | 39 |
| [docker/gatesvr/Dockerfile](/docker/gatesvr/Dockerfile) | Docker | 33 | 20 | 19 | 72 |
| [docker/upstream/Dockerfile](/docker/upstream/Dockerfile) | Docker | 36 | 19 | 18 | 73 |
| [docker部署.md](/docker%E9%83%A8%E7%BD%B2.md) | Markdown | 52 | 0 | 22 | 74 |
| [gatesvr.log](/gatesvr.log) | Log | 14,297 | 0 | 1 | 14,298 |
| [go.mod](/go.mod) | Go Module File | 28 | 0 | 5 | 33 |
| [go.sum](/go.sum) | Go Checksum File | 76 | 0 | 1 | 77 |
| [internal/backup/backup\_manager.go](/internal/backup/backup_manager.go) | Go | 388 | 53 | 77 | 518 |
| [internal/backup/data\_validator.go](/internal/backup/data_validator.go) | Go | 216 | 35 | 66 | 317 |
| [internal/backup/failover.go](/internal/backup/failover.go) | Go | 369 | 56 | 82 | 507 |
| [internal/backup/heartbeat.go](/internal/backup/heartbeat.go) | Go | 546 | 93 | 119 | 758 |
| [internal/backup/interfaces.go](/internal/backup/interfaces.go) | Go | 61 | 50 | 47 | 158 |
| [internal/backup/sync\_receiver.go](/internal/backup/sync_receiver.go) | Go | 326 | 58 | 71 | 455 |
| [internal/backup/sync\_service.go](/internal/backup/sync_service.go) | Go | 394 | 48 | 73 | 515 |
| [internal/backup/types.go](/internal/backup/types.go) | Go | 97 | 12 | 15 | 124 |
| [internal/client/client.go](/internal/client/client.go) | Go | 693 | 119 | 167 | 979 |
| [internal/config/config.go](/internal/config/config.go) | Go | 249 | 27 | 33 | 309 |
| [internal/gateway/config.go](/internal/gateway/config.go) | Go | 28 | 9 | 10 | 47 |
| [internal/gateway/handlers.go](/internal/gateway/handlers.go) | Go | 323 | 58 | 72 | 453 |
| [internal/gateway/http\_handlers.go](/internal/gateway/http_handlers.go) | Go | 392 | 288 | 57 | 737 |
| [internal/gateway/ordered\_sender.go](/internal/gateway/ordered_sender.go) | Go | 124 | 18 | 32 | 174 |
| [internal/gateway/overload\_protector.go](/internal/gateway/overload_protector.go) | Go | 285 | 46 | 61 | 392 |
| [internal/gateway/performance.go](/internal/gateway/performance.go) | Go | 376 | 47 | 88 | 511 |
| [internal/gateway/server.go](/internal/gateway/server.go) | Go | 195 | 47 | 60 | 302 |
| [internal/gateway/server\_connection.go](/internal/gateway/server_connection.go) | Go | 273 | 47 | 54 | 374 |
| [internal/gateway/server\_push.go](/internal/gateway/server_push.go) | Go | 37 | 4 | 9 | 50 |
| [internal/gateway/server\_start.go](/internal/gateway/server_start.go) | Go | 106 | 31 | 32 | 169 |
| [internal/gateway/simple\_performance.go](/internal/gateway/simple_performance.go) | Go | 47 | 4 | 6 | 57 |
| [internal/gateway/simple\_tracker.go](/internal/gateway/simple_tracker.go) | Go | 77 | 12 | 14 | 103 |
| [internal/gateway/unicast\_service.go](/internal/gateway/unicast_service.go) | Go | 185 | 47 | 52 | 284 |
| [internal/gateway/upstream\_router.go](/internal/gateway/upstream_router.go) | Go | 55 | 10 | 15 | 80 |
| [internal/message/codec.go](/internal/message/codec.go) | Go | 237 | 44 | 60 | 341 |
| [internal/session/manager.go](/internal/session/manager.go) | Go | 517 | 93 | 120 | 730 |
| [internal/session/notify\_ordering.go](/internal/session/notify_ordering.go) | Go | 118 | 28 | 34 | 180 |
| [internal/session/ordered\_queue.go](/internal/session/ordered_queue.go) | Go | 245 | 37 | 53 | 335 |
| [internal/session/queue\_config.go](/internal/session/queue_config.go) | Go | 25 | 3 | 7 | 35 |
| [internal/session/session.go](/internal/session/session.go) | Go | 388 | 110 | 110 | 608 |
| [internal/upstream/manager.go](/internal/upstream/manager.go) | Go | 119 | 16 | 37 | 172 |
| [internal/upstream/server.go](/internal/upstream/server.go) | Go | 571 | 66 | 123 | 760 |
| [internal/upstream/services.go](/internal/upstream/services.go) | Go | 137 | 16 | 38 | 191 |
| [internal/upstream/unicast\_client.go](/internal/upstream/unicast_client.go) | Go | 124 | 13 | 32 | 169 |
| [optimize\_queue\_analysis.md](/optimize_queue_analysis.md) | Markdown | 116 | 0 | 31 | 147 |
| [pkg/metrics/metrics.go](/pkg/metrics/metrics.go) | Go | 150 | 33 | 38 | 221 |
| [proto/message.pb.go](/proto/message.pb.go) | Go | 843 | 40 | 136 | 1,019 |
| [proto/upstream.pb.go](/proto/upstream.pb.go) | Go | 801 | 31 | 121 | 953 |
| [proto/upstream\_grpc.pb.go](/proto/upstream_grpc.pb.go) | Go | 218 | 66 | 32 | 316 |
| [scripts/generate\_certs.bat](/scripts/generate_certs.bat) | Batch | 40 | 7 | 9 | 56 |
| [scripts/generate\_certs.sh](/scripts/generate_certs.sh) | Shell Script | 18 | 7 | 9 | 34 |
| [scripts/lancy.md](/scripts/lancy.md) | Markdown | 82 | 0 | 32 | 114 |
| [scripts/start-backup.bat](/scripts/start-backup.bat) | Batch | 5 | 0 | 3 | 8 |
| [scripts/start-primary.bat](/scripts/start-primary.bat) | Batch | 5 | 0 | 3 | 8 |
| [scripts/test\_grpc\_unicast.ps1](/scripts/test_grpc_unicast.ps1) | PowerShell | 50 | 7 | 8 | 65 |
| [scripts/test\_performance.sh](/scripts/test_performance.sh) | Shell Script | 169 | 24 | 33 | 226 |
| [scripts/test\_unicast.sh](/scripts/test_unicast.sh) | Shell Script | 0 | 0 | 1 | 1 |
| [test-config.yaml](/test-config.yaml) | YAML | 31 | 1 | 5 | 37 |
| [拆分上游服务器.md](/%E6%8B%86%E5%88%86%E4%B8%8A%E6%B8%B8%E6%9C%8D%E5%8A%A1%E5%99%A8.md) | Markdown | 125 | 0 | 59 | 184 |

[Summary](results.md) / Details / [Diff Summary](diff.md) / [Diff Details](diff-details.md)