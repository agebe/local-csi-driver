FROM golang:1.19.8 as builder

WORKDIR /app
COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -a -o local-csi-driver main.go

FROM alpine:latest  
RUN apk --no-cache add ca-certificates

WORKDIR /root/
COPY --from=builder /app/local-csi-driver .

CMD ["./local-csi-driver"]
