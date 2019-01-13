FROM alpine

RUN ["apk", "--no-cache", "add", "curl"]

VOLUME ["/etc/jsonmon"]
WORKDIR /etc/jsonmon
ENV HOST=[::]
COPY jsonmon /usr/local/bin/

USER nobody
CMD ["jsonmon", "config.yml"]
EXPOSE 3000
