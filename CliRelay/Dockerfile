# 镜像源配置参数
ARG DOCKER_REGISTRY=
ARG APK_MIRROR=

FROM ${DOCKER_REGISTRY:-}golang:1.26-alpine AS builder

# 配置 Alpine apk 镜像源（如果指定）
ARG APK_MIRROR
RUN if [ -n "$APK_MIRROR" ]; then \
        sed -i "s@dl-cdn.alpinelinux.org@$APK_MIRROR@g" /etc/apk/repositories; \
    fi

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . .

ARG VERSION=dev
ARG COMMIT=none
ARG BUILD_DATE=unknown

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w -X 'main.Version=${VERSION}' -X 'main.Commit=${COMMIT}' -X 'main.BuildDate=${BUILD_DATE}'" -o ./CLIProxyAPI ./cmd/server/

# 镜像源配置参数
ARG DOCKER_REGISTRY=
ARG APK_MIRROR=

FROM ${DOCKER_REGISTRY:-}alpine:3.22.0

# 配置 Alpine apk 镜像源（如果指定）
ARG APK_MIRROR
RUN if [ -n "$APK_MIRROR" ]; then \
        sed -i "s@dl-cdn.alpinelinux.org@$APK_MIRROR@g" /etc/apk/repositories; \
    fi

RUN apk add --no-cache tzdata

RUN mkdir /CLIProxyAPI

COPY --from=builder ./app/CLIProxyAPI /CLIProxyAPI/CLIProxyAPI

COPY config.example.yaml /CLIProxyAPI/config.example.yaml

WORKDIR /CLIProxyAPI

EXPOSE 8317

ENV TZ=Asia/Shanghai

RUN cp /usr/share/zoneinfo/${TZ} /etc/localtime && echo "${TZ}" > /etc/timezone

CMD ["./CLIProxyAPI"]