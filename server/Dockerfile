FROM golang:1.26.6-alpine AS build

WORKDIR /src
COPY go.mod ./
COPY go.sum ./
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/partner-watch-server ./cmd/partner-watch-server
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/partner-watch-admin ./cmd/partner-watch-admin
RUN mkdir -p /out/data

FROM gcr.io/distroless/static-debian13:nonroot
COPY --from=build /out/partner-watch-server /partner-watch-server
COPY --from=build /out/partner-watch-admin /partner-watch-admin
COPY --from=build --chown=65532:65532 /out/data /var/lib/partner-watch
EXPOSE 8080
ENTRYPOINT ["/partner-watch-server"]
