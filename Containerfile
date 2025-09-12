FROM docker.io/library/golang:1.24.6 AS builder
COPY . /src
WORKDIR /src
ENV CGO_ENABLED=0
RUN go build -o manager -ldflags="-s -w"

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /src/manager /usr/bin/manager
ENTRYPOINT ["/usr/bin/manager"]
