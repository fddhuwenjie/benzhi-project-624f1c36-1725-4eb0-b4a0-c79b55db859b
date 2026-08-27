# 广播母版响度交付门禁

这是一个面向广播音频工程师与独立质量复核员的响度合规交付工作台。系统将节目母版交付案件从建档、标准冻结、分段与测量证据登记，推进到确定性规则判定、偏差整改复测、独立复核，并在批准后生成带 SHA-256 校验值的不可变交付清单。服务由 Go 直接托管原生 HTML、CSS 和 JavaScript 单页界面，业务数据保存在本地事务 JSON 快照中。

## 构建

项目使用 Go 标准工具链，无需 Node 构建链或外部数据库。

```text
go build ./cmd/mastergate
```

## 运行

默认监听 `127.0.0.1:19081`，也可以通过 `-addr=127.0.0.1:<port>` 或 `PORT` 环境变量配置端口。服务只接受回环地址，案件数据默认写入系统临时目录，也可用 `-storage` 指定快照路径。

```text
go run ./cmd/mastergate -addr=127.0.0.1:19081 -storage=./mastergate.json
```

浏览器访问 `http://127.0.0.1:19081/`，在工作台中依次完成案件建档、冻结前元数据修订、基线冻结、节目分段覆盖预检、整体及分段测量批次导入、规则判定、偏差整改复测和独立复核。基线冻结且尚无测量时，可从分段表预填修订标题、边界、声道及证据字段，或在无测量、规则和偏差引用时撤销误建分段；每次变更都会重新校验完整分段集合并写入审计事件。所有写操作都携带 `request_id` 与 `expected_revision`；重复请求会重放首次响应，陈旧修订号会返回冲突。同节目编号、制作版本和母版摘要的草稿会被原子查重拦截。

案件详情按时间排序显示分段覆盖、缺口和重叠，判定前会再次执行完整性预检。测量按范围保留不可覆盖的 `supersedes_id` 版本链；每轮判定保存稳定 SHA-256 规则快照，整改复测只更新偏差所引用的规则。偏差队列按测量范围分组，并按真峰值、集成响度、响度范围的稳定优先级排序；同组开放偏差可登记共同整改，同组待复测偏差可共享一份新测量并按各自 `rule_code` 独立重评。

独立复核标签随案件详情加载绑定当前 revision 的证据矩阵，按 `baseline`、`evidence`、`rules`、`remediation` 展示稳定检查代码、中文阻断原因和测量、快照、偏差或事件定位引用。批准提交会在事务中用同一完备性规则重新计算；陈旧 revision 返回当前修订号和最新阻断项。复核仍要求四项唯一批注，批准后生成不可变清单，拒绝后案件进入只读终态。

## 测试

```text
go test ./...
```

完整 HTTP 自检会临时创建快照，使用真实回环服务驱动一次失败整改后批准的流程，验证清单并主动退出：

```text
go run ./cmd/mastergate -selftest -addr=127.0.0.1:19081
```

主要 JSON 端点包括 `POST /api/cases`、`GET /api/cases`、`GET /api/cases/{case_id}`、`POST /api/cases/{case_id}/metadata`、`POST /api/cases/{case_id}/freeze`、`POST /api/cases/{case_id}/segments`、`POST /api/cases/{case_id}/segment-revisions`、`POST /api/cases/{case_id}/segment-withdrawals`、`GET /api/cases/{case_id}/preflight`、`POST /api/cases/{case_id}/measurements`、`POST /api/cases/{case_id}/measurement-batches`、`POST /api/cases/{case_id}/evaluate`、`POST /api/cases/{case_id}/corrections`、`POST /api/cases/{case_id}/joint-corrections`、`POST /api/cases/{case_id}/retests`、`POST /api/cases/{case_id}/joint-retests`、`GET /api/cases/{case_id}/readiness`、`POST /api/cases/{case_id}/review`、`GET /api/cases/{case_id}/timeline` 和 `GET /api/cases/{case_id}/verify`。

`GET /api/cases` 支持 `program_code`、`state`、`engineer_id`、`approved_from`、`approved_to` 查询参数，其中批准时间使用 RFC3339。`GET /api/cases/{case_id}/verify` 返回规范化清单载荷、封存摘要、事件链尾以及逐字段校验结果；发现不一致时只报告具体失败位置，不会修复或改写封存数据。
