FROM golang:1.25-bookworm

WORKDIR /app

COPY . .

RUN make ci

CMD ["/bin/sh"]
