FROM golang:1.10.1-alpine3.7 as builder

# Add build tools
RUN apk update && \
    apk add --no-cache git gcc musl-dev glide

ENV SRC_DIR=/go/src/github.com/containership/infrastructure-controller/

WORKDIR /app

# Glide install before adding rest of source so we can cache the resulting
# vendor dir
COPY glide.yaml glide.lock $SRC_DIR
RUN cd $SRC_DIR && \
        glide install -v

# Add the source code
COPY . $SRC_DIR

# Build it:
ARG GIT_DESCRIBE
ARG GIT_COMMIT
RUN cd $SRC_DIR && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
        go build -ldflags \
        "-X github.com/containership/infrastructure-controller/pkg/buildinfo.gitDescribe=${GIT_DESCRIBE} \
        -X github.com/containership/infrastructure-controller/pkg/buildinfo.gitCommit=${GIT_COMMIT} \
        -X github.com/containership/infrastructure-controller/pkg/buildinfo.unixTime=`date '+%s'` \
        -w" \
        -a -tags netgo \
        -o infrastructure-controller cmd/main.go && \
    cp infrastructure-controller /app/


# Create Docker image of just the binary
FROM scratch as runner
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /app/infrastructure-controller .

CMD ["./infrastructure-controller"]
