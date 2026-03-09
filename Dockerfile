# By default, Jule generates executables that rely on glibc, so Alpine can't be used.
# Jule can generate static binaries, but it breaks std packages like std/net.

FROM debian:bookworm-slim

RUN apt-get update \
    # ca-certificates enables secure HTTPS downloads
    && apt-get install -y --no-install-recommends wget unzip clang ca-certificates \
    && rm -rf /var/lib/apt/lists/* \
    && wget -O jule.zip https://github.com/julelang/jule/releases/download/jule0.2.0/jule0.2.0-linux-amd64.zip \
    && unzip jule.zip \
    && rm -rf jule.zip __MACOSX

# So that you can directly call julec from the backend
ENV PATH=$PATH:/jule/bin
