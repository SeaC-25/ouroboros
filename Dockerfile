# GateMesh 参赛镜像（doc/gatemesh-architecture-v1.md §S7）
#
# 多阶段构建：
#   stage 1 (builder): Go 1.26 编译 ouro 静态二进制（modernc.org/sqlite 纯 Go，无 CGO）
#   stage 2 (runtime): distroless base-debian12 nonroot —— 无 shell、最小攻击面
#
# 内容：ouro（GateMesh 六包 + agent run）+ fastsec + KB 资产
# 入口：ouro agent run --objective ... --scope-policy ... --auto-accept

FROM golang:1.26 AS builder

WORKDIR /src
ENV GOPROXY=https://goproxy.cn,direct
COPY go.mod go.sum ./
RUN go mod download

COPY cmd/ cmd/
COPY internal/ internal/
COPY configs/ configs/

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath \
    -ldflags="-s -w" \
    -o /out/ouro ./cmd/ouro

# ---- runtime ----
# debian:12-slim locally trimmed (gcr.io distroless unreachable from this
# network); binaries are static (CGO_ENABLED=0) so no runtime libs needed.
FROM debian:12-slim
RUN rm -rf /bin/sh /bin/bash /bin/dash /usr/bin/apt* /usr/bin/dpkg* 2>/dev/null || true

WORKDIR /workspace
# ouro 主二进制（agent run + 全部 CLI）
COPY --from=builder /out/ouro /usr/local/bin/ouro

# fastsec（KaliMCP 出品的 Go 扫描引擎，交叉编译产物）
COPY fastsec-linux-amd64 /usr/local/bin/fastsec

# KB 资产（265 chunks 攻击链/writeup/配方知识库）
COPY assets/kb_vectors.db /opt/ouro/kb_vectors.db

# 工作区与报告卷
VOLUME ["/workspace", "/out"]

USER 65532

ENTRYPOINT ["/usr/local/bin/ouro"]
CMD ["agent", "run", "--help"]
