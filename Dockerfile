FROM alpine

LABEL Name=jsonmon Version=3.1.10

RUN ["apk", "--no-cache", "add", "curl"]

VOLUME ["/etc/jsonmon"]
WORKDIR /etc/jsonmon
ENV HOST=[::]
COPY jsonmon /usr/bin

USER nobody
CMD ["jsonmon", "config.yml"]
EXPOSE 3000
