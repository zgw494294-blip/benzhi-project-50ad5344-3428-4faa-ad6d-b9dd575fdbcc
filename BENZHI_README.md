# BENZHI_README

## 项目说明
- 项目：benzhi-project-50ad5344-3428-4faa-ad6d-b9dd575fdbcc
- 项目用途：CartonProof is a standard-library HTTP JSON service for immutable food-carton artwork compliance reviews. Default startup now performs a bounded HTTP readiness probe and exits successfully, while --serve retains persistent service operation and --smoke exercises the complete approval workflow.
- Go 工具链：`golang:1.22.0`
- 前端工具链：无

## 标准构建、运行和测试命令
进入容器后执行：
cd '/app' && GOTOOLCHAIN=local go build ./...
cd /app && GOTOOLCHAIN=local go run ./cmd/cartonproof
cd '/app' && GOTOOLCHAIN=local go test ./...

## Docker 构建和进入容器
chmod +x build_benzhi_docker.sh
./build_benzhi_docker.sh benzhi-project-50ad5344-3428-4faa-ad6d-b9dd575fdbcc-amd64 linux/amd64
./build_benzhi_docker.sh benzhi-project-50ad5344-3428-4faa-ad6d-b9dd575fdbcc-arm64 linux/arm64
docker run -it benzhi-project-50ad5344-3428-4faa-ad6d-b9dd575fdbcc-amd64:latest

## 题目验证命令
1. 预期退出码 0：`go test ./...`
2. 预期退出码 0：`go run ./cmd/cartonproof --smoke`
