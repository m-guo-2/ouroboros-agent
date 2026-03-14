FROM golang:1.24-alpine AS builder

WORKDIR /src
COPY shared/ /src/shared/
COPY agent/ /src/agent/

WORKDIR /src/agent
RUN CGO_ENABLED=1 GOOS=linux go build -o /agent-server ./cmd/agent/...

FROM alpine:3.20

RUN apk add --no-cache \
    chromium \
    nss \
    freetype \
    harfbuzz \
    ca-certificates \
    ttf-freefont \
    font-noto-cjk \
    && rm -rf /var/cache/apk/*

ENV CHROME_BIN=/usr/bin/chromium-browser
ENV CHROME_PATH=/usr/lib/chromium/

COPY --from=builder /agent-server /usr/local/bin/agent-server

WORKDIR /app
EXPOSE 8080

CMD ["agent-server"]
