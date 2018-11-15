# --- Выпуск, используя Alpine ----
FROM golang:alpine as builder
RUN apk update && apk add git 

WORKDIR /src
COPY go /src
RUN go get -d -v
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -installsuffix cgo -ldflags="-w -s" -v -o /src/api

FROM alpine:latest

RUN apk --update add jpegoptim optipng libwebp-tools imagemagick && rm -rf /var/cache/apk/*

WORKDIR /src
COPY --from=builder /src/api /src/api

EXPOSE 8080
USER 33

ENTRYPOINT ["/src/api"]
