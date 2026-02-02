FROM alpine:3.19

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

COPY proxy-rfc2217 /app/proxy-rfc2217

EXPOSE 2217 8080

ENTRYPOINT ["/app/proxy-rfc2217"]
