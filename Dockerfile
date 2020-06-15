ARG ARCH=amd64

# build stage
FROM golang:1.13 AS builder
RUN mkdir -p /go/src/repo
WORKDIR /go/src/repo
COPY . ./
RUN go mod download
RUN go mod verify
RUN CGO_ENABLED=0 GOOS=linux GOARCH=$ARCH go build -a -o /app .


# final stage
FROM alpine:latest
RUN apk --no-cache add ca-certificates
COPY --from=builder /app ./
COPY --from=builder /go/src/repo/config ./config
COPY templates ./templates
RUN chmod +x ./app
ENTRYPOINT ["./app"]
EXPOSE 5555
