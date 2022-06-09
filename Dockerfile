FROM golang:1.18-alpine3.16 as builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . ./
RUN go build -o /refractor ./cmd

FROM alpine:3.16

COPY --from=builder /refractor /bin/
COPY --from=builder /build/refractor.yaml /config/refractor.yaml

ENTRYPOINT ["/bin/refractor"]
CMD ["-config", "/config/refractor.yaml"]
