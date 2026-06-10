FROM golang:1.25-alpine AS builder
RUN apk add --no-cache git
ENV GOPROXY=https://goproxy.cn,direct
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
ENV GOWORK=off
RUN go mod tidy -e && \
    CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o server ./cmd/main.go

FROM alpine:latest
RUN apk --no-cache add ca-certificates tzdata
ENV TZ=Asia/Shanghai
WORKDIR /app
COPY --from=builder /app/server .
EXPOSE 8082
CMD ["./server"]
