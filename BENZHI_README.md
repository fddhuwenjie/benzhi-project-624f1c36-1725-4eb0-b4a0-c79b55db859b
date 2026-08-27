# BENZHI_README

## 项目说明
- 项目：benzhi-project-624f1c36-1725-4eb0-b4a0-c79b55db859b
- 项目用途：广播母版响度交付门禁提供从案件建档、标准冻结、分段与测量证据登记，到规则判定、偏差整改复测、独立复核和不可变交付清单校验的完整中文浏览器工作台。
- Go 工具链：`golang:1.22`
- 前端工具链：原生 HTML、CSS 和 JavaScript，由 Go 服务直接提供

## 项目描述
- 项目名称：广播母版响度交付门禁
- 项目介绍：面向广播技术制作团队的响度合规交付工作台，将单个节目母版从交付案件建档、测量证据登记、自动规则判定、偏差整改复测推进到独立复核，并生成可校验的不可变交付清单。
- 项目概述：面向广播技术制作团队的响度合规交付工作台，将单个节目母版从交付案件建档、测量证据登记、自动规则判定、偏差整改复测推进到独立复核，并生成可校验的不可变交付清单。
- 核心工作流：音频工程师创建节目母版交付案件并冻结适用的响度标准，登记节目分段与测量证据后执行确定性合规判定；不合格项进入偏差整改并提交新版本复测，全部合格后由不同人员独立复核，最终批准并封存可验证的交付清单，或以拒绝结论结束案件。
- 对外接口：Go 服务直接提供原生 HTML、CSS 和 JavaScript 的单页浏览器工作台及仅供该页面使用的同源 JSON 端点；页面包含案件状态带、节目分段表、测量录入区、规则结果与偏差队列、复核面板、审计时间线和终态交付清单校验视图，不引入 Node 构建链。

## 标准构建、运行和测试命令
进入容器后执行：
cd '/app' && GOTOOLCHAIN=local go build ./...

cd /app && GOTOOLCHAIN=local go run ./cmd/mastergate -selftest -addr=127.0.0.1:19081

cd '/app' && GOTOOLCHAIN=local go test ./...

## Docker 构建和进入容器
chmod +x build_benzhi_docker.sh

./build_benzhi_docker.sh benzhi-project-624f1c36-1725-4eb0-b4a0-c79b55db859b-amd64 linux/amd64

./build_benzhi_docker.sh benzhi-project-624f1c36-1725-4eb0-b4a0-c79b55db859b-arm64 linux/arm64

docker run -it benzhi-project-624f1c36-1725-4eb0-b4a0-c79b55db859b-amd64:latest

## 题目验证命令
1. 预期退出码 0：`go test ./...`
2. 预期退出码 0：`go run ./cmd/mastergate -selftest -addr=127.0.0.1:19081`
