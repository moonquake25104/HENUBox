# syntax=docker/dockerfile:1
FROM golang:1.25-alpine AS build
WORKDIR /src
RUN apk add --no-cache ca-certificates tzdata
ARG GOPROXY=https://proxy.golang.org,direct
ENV GOPROXY=${GOPROXY}
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/kairos-api ./cmd/api

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
ENV TZ=Asia/Shanghai
WORKDIR /app
COPY --from=build /out/kairos-api /usr/local/bin/kairos-api
RUN mkdir -p /data \
    && addgroup -S kairos \
    && adduser -S -G kairos kairos \
    && chown -R kairos:kairos /data
EXPOSE 8080
USER kairos
ENTRYPOINT ["kairos-api"]
