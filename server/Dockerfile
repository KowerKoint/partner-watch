FROM golang:1.26.6-alpine AS build

WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/partner-watch-server ./cmd/partner-watch-server

FROM gcr.io/distroless/static-debian13:nonroot
COPY --from=build /out/partner-watch-server /partner-watch-server
EXPOSE 8080
ENTRYPOINT ["/partner-watch-server"]
