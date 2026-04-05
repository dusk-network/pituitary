FROM golang:1.26-bookworm

WORKDIR /app

COPY . .

RUN make ci

CMD ["/bin/sh"]
