FROM golang:1.18-alpine3.16 as builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . ./
RUN go build -o /shatter ./cmd

FROM alpine:3.16

COPY --from=builder /shatter /bin/
COPY --from=builder /build/shatter.yaml /config/shatter.yaml

ENTRYPOINT ["/bin/shatter"]
CMD ["-config", "/config/shatter.yaml"]
