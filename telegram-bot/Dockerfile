FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/lynx-telegram ./cmd/lynx-telegram

FROM alpine:3.21
RUN apk add --no-cache ca-certificates && adduser -D -H -u 10001 app
COPY --from=build /out/lynx-telegram /usr/local/bin/lynx-telegram
USER app
EXPOSE 8081
ENTRYPOINT ["/usr/local/bin/lynx-telegram"]
