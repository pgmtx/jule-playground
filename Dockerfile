FROM alpine:latest

RUN apk add --no-cache clang
RUN wget -O jule.zip https://github.com/julelang/jule/releases/download/jule0.2.0/jule0.2.0-linux-amd64.zip
RUN unzip jule.zip && rm -rf jule.zip __MACOSX

CMD ["sh"]
