FROM golang:1.25-bookworm

RUN apt-get update && \
    apt-get install -y --no-install-recommends build-essential make && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY . .

RUN make ci

CMD ["/bin/sh"]
