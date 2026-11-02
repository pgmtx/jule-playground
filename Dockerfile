# By default, Jule generates executables that rely on glibc, so Alpine can't be used.
# Jule can generate static binaries, but it breaks std packages like std/net.

FROM debian:trixie-slim

RUN apt-get update \
    # ca-certificates enables secure HTTPS downloads
    && apt-get install -y --no-install-recommends wget git clang ca-certificates xz-utils \
    && rm -rf /var/lib/apt/lists/* \
    && wget -O jule.tar.xz https://github.com/julelang/jule/releases/download/jule0.3.0/jule0.3.0-linux-amd64.tar.xz \
    && tar -xf jule.tar.xz \
    && rm -rf jule.tar.xz \
    && git clone https://github.com/julelang/julefmt.git /julefmt \
    && cd /julefmt \
    && git checkout 84bfc1f \
    && /jule/bin/julec build --opt-deadcode -o julefmt ./src

# So that you can directly call julec from the backend
ENV PATH=$PATH:/jule/bin:/julefmt
