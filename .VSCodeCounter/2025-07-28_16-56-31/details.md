# Details

Date : 2025-07-28 16:56:31

Directory d:\\25cxx\\try\\v1

Total : 49 files,  11217 codes, 1119 comments, 2254 blanks, all 14590 lines

[Summary](results.md) / Details / [Diff Summary](diff.md) / [Diff Details](diff-details.md)

## Files
| filename | language | code | comment | blank | total |
| :--- | :--- | ---: | ---: | ---: | ---: |
| [.idea/modules.xml](/.idea/modules.xml) | XML | 8 | 0 | 0 | 8 |
| [.idea/v1.iml](/.idea/v1.iml) | XML | 9 | 0 | 0 | 9 |
| [GRPC\_UNICAST\_SUMMARY.md](/GRPC_UNICAST_SUMMARY.md) | Markdown | 116 | 0 | 38 | 154 |
| [LATENCY\_MONITORING.md](/LATENCY_MONITORING.md) | Markdown | 160 | 0 | 46 | 206 |
| [Makefile](/Makefile) | Makefile | 201 | 35 | 35 | 271 |
| [OPTIMIZATION\_SUMMARY.md](/OPTIMIZATION_SUMMARY.md) | Markdown | 309 | 0 | 80 | 389 |
| [QUICKSTART.md](/QUICKSTART.md) | Markdown | 266 | 0 | 75 | 341 |
| [build.bat](/build.bat) | Batch | 45 | 0 | 8 | 53 |
| [cmd/client/main.go](/cmd/client/main.go) | Go | 488 | 36 | 92 | 616 |
| [cmd/gatesvr/main.go](/cmd/gatesvr/main.go) | Go | 137 | 17 | 27 | 181 |
| [cmd/gencerts/main.go](/cmd/gencerts/main.go) | Go | 103 | 8 | 19 | 130 |
| [cmd/monitor/main.go](/cmd/monitor/main.go) | Go | 376 | 9 | 67 | 452 |
| [cmd/upstream/main.go](/cmd/upstream/main.go) | Go | 81 | 11 | 20 | 112 |
| [gatesvr/proto/message.pb.go](/gatesvr/proto/message.pb.go) | Go | 616 | 29 | 100 | 745 |
| [gatesvr/proto/upstream.pb.go](/gatesvr/proto/upstream.pb.go) | Go | 1,299 | 47 | 197 | 1,543 |
| [gatesvr/proto/upstream\_grpc.pb.go](/gatesvr/proto/upstream_grpc.pb.go) | Go | 245 | 70 | 36 | 351 |
| [go.mod](/go.mod) | Go Module File | 26 | 0 | 5 | 31 |
| [go.sum](/go.sum) | Go Checksum File | 66 | 0 | 1 | 67 |
| [internal/client/client.go](/internal/client/client.go) | Go | 422 | 65 | 98 | 585 |
| [internal/gateway/config.go](/internal/gateway/config.go) | Go | 14 | 5 | 5 | 24 |
| [internal/gateway/handlers.go](/internal/gateway/handlers.go) | Go | 140 | 22 | 28 | 190 |
| [internal/gateway/http\_handlers.go](/internal/gateway/http_handlers.go) | Go | 106 | 13 | 20 | 139 |
| [internal/gateway/performance.go](/internal/gateway/performance.go) | Go | 283 | 38 | 58 | 379 |
| [internal/gateway/push\_client.go](/internal/gateway/push_client.go) | Go | 292 | 43 | 79 | 414 |
| [internal/gateway/server.go](/internal/gateway/server.go) | Go | 126 | 28 | 41 | 195 |
| [internal/gateway/server\_connection.go](/internal/gateway/server_connection.go) | Go | 188 | 35 | 43 | 266 |
| [internal/gateway/server\_push.go](/internal/gateway/server_push.go) | Go | 78 | 8 | 18 | 104 |
| [internal/gateway/server\_start.go](/internal/gateway/server_start.go) | Go | 75 | 11 | 23 | 109 |
| [internal/gateway/server\_upstream.go](/internal/gateway/server_upstream.go) | Go | 80 | 18 | 21 | 119 |
| [internal/gateway/server\_utils.go](/internal/gateway/server_utils.go) | Go | 26 | 6 | 9 | 41 |
| [internal/gateway/unicast\_service.go](/internal/gateway/unicast_service.go) | Go | 53 | 5 | 14 | 72 |
| [internal/message/codec.go](/internal/message/codec.go) | Go | 192 | 41 | 51 | 284 |
| [internal/session/cache.go](/internal/session/cache.go) | Go | 260 | 40 | 62 | 362 |
| [internal/session/manager.go](/internal/session/manager.go) | Go | 375 | 78 | 96 | 549 |
| [internal/session/session.go](/internal/session/session.go) | Go | 223 | 58 | 69 | 350 |
| [internal/upstream/broadcast\_manager.go](/internal/upstream/broadcast_manager.go) | Go | 299 | 42 | 63 | 404 |
| [internal/upstream/server.go](/internal/upstream/server.go) | Go | 420 | 61 | 102 | 583 |
| [internal/upstream/unicast\_client.go](/internal/upstream/unicast_client.go) | Go | 122 | 14 | 30 | 166 |
| [pkg/metrics/metrics.go](/pkg/metrics/metrics.go) | Go | 101 | 26 | 27 | 154 |
| [proto/message.pb.go](/proto/message.pb.go) | Go | 616 | 29 | 100 | 745 |
| [proto/upstream.pb.go](/proto/upstream.pb.go) | Go | 1,299 | 47 | 197 | 1,543 |
| [proto/upstream\_grpc.pb.go](/proto/upstream_grpc.pb.go) | Go | 245 | 70 | 36 | 351 |
| [scripts/generate\_certs.bat](/scripts/generate_certs.bat) | Batch | 40 | 7 | 9 | 56 |
| [scripts/generate\_certs.sh](/scripts/generate_certs.sh) | Shell Script | 18 | 7 | 9 | 34 |
| [scripts/test\_grpc\_unicast.go](/scripts/test_grpc_unicast.go) | Go | 90 | 5 | 13 | 108 |
| [scripts/test\_grpc\_unicast.ps1](/scripts/test_grpc_unicast.ps1) | PowerShell | 50 | 7 | 8 | 65 |
| [scripts/test\_performance.sh](/scripts/test_performance.sh) | Shell Script | 169 | 24 | 33 | 226 |
| [scripts/test\_unicast.ps1](/scripts/test_unicast.ps1) | PowerShell | 158 | 1 | 26 | 185 |
| [scripts/test\_unicast.sh](/scripts/test_unicast.sh) | Shell Script | 106 | 3 | 20 | 129 |

[Summary](results.md) / Details / [Diff Summary](diff.md) / [Diff Details](diff-details.md)