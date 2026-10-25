FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY *.go ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/ghreport .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates \
    && adduser -D -H -u 10001 ghreport \
    && mkdir -p /work \
    && chown ghreport:ghreport /work
COPY --from=build /out/ghreport /usr/local/bin/ghreport
WORKDIR /work
USER ghreport
ENTRYPOINT ["ghreport"]
