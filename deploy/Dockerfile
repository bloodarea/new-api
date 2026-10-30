# Minimal runtime image: the frontend is embedded in the binary (go:embed
# web/dist), so only the prebuilt binary + licenses are needed here.
# The heavy toolchain lives outside this repo; see ../build.sh.
#
# BASE_REGISTRY is the registry prefix in front of the pinned digest. It defaults
# to the China mirror used by the local build; CI passes an empty value so the
# GitHub runner pulls from docker.io. Same digest either way.
ARG BASE_REGISTRY=docker.m.daocloud.io/library/
FROM ${BASE_REGISTRY}debian:bookworm-slim@sha256:f06537653ac770703bc45b4b113475bd402f451e85223f0f2837acbf89ab020a

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates tzdata libasan8 wget \
    && rm -rf /var/lib/apt/lists/* \
    && update-ca-certificates

COPY new-api /new-api
COPY licenses /licenses
EXPOSE 3000
WORKDIR /data
ENTRYPOINT ["/new-api"]
