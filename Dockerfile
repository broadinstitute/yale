ARG GO_VERSION='1.20'
ARG ALPINE_VERSION='3.17'

FROM golang:${GO_VERSION}-alpine${ALPINE_VERSION} as build
WORKDIR /build
ENV CGO_ENABLED=0
ENV GO111MODULE=on
ENV GOBIN=/bin
COPY . .
RUN go test ./... && go build -o /bin/ ./cmd/...

FROM alpine:${ALPINE_VERSION} as runtime
ENV APP_NAME=yale
COPY --from=build /bin/* /bin/
ENTRYPOINT [ "sh", "-c", "/bin/${APP_NAME}" ]
