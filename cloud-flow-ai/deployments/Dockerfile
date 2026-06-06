FROM golang:1.22-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o cloud-flow-ai ./cmd

FROM alpine:latest
RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

COPY --from=builder /app/cloud-flow-ai .
COPY configs/config.yaml /app/configs/config.yaml

EXPOSE 8082

CMD ["./cloud-flow-ai"]
