FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o my-monitor .

FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata iputils
ENV TZ=Asia/Shanghai
WORKDIR /app
COPY --from=builder /app/my-monitor .
COPY config.example.yaml config.yaml
CMD ["./my-monitor"]
