FROM golang:1.25.5-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY database ./database
RUN CGO_ENABLED=0 go build -trimpath -o /out/server ./cmd/server && \
    CGO_ENABLED=0 go build -trimpath -o /out/seed ./cmd/seed

FROM alpine:3.23.0
RUN apk add --no-cache ca-certificates && adduser -D -u 10001 app
USER app
COPY --from=build /out/server /usr/local/bin/server
COPY --from=build /out/seed /usr/local/bin/seed
COPY --chown=app:app data/public/seed.json /data/public/seed.json
EXPOSE 8080
ENTRYPOINT ["server"]
