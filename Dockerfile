# HyperFleet E2E Testing Framework
#
# Build: podman build -t quay.io/hyperfleet/hyperfleet-e2e:latest .
# Build with commit: podman build --build-arg GIT_COMMIT=$(git rev-parse HEAD) -t quay.io/hyperfleet/hyperfleet-e2e:latest .
# Run:   podman run --rm -e HYPERFLEET_API_URL=<url> quay.io/hyperfleet/hyperfleet-e2e:latest test

ARG BASE_IMAGE=registry.access.redhat.com/ubi9/go-toolset

# Build stage (go-toolset already ships make)
FROM registry.access.redhat.com/ubi9/go-toolset:9.8-1792010711 AS builder

# go-toolset builds as non-root user 1001, which only needs to write bin/ under /build
USER root
WORKDIR /build
RUN chown 1001:0 /build
USER 1001

# Download Go modules in their own layer so code-only changes reuse it
COPY go.mod go.sum ./
RUN go mod download

# Build ginkgo CLI (version-locked via tools/go.mod)
COPY tools/go.mod tools/go.sum tools/
RUN cd tools && go build -o /build/bin/ginkgo "github.com/onsi/ginkgo/v2/ginkgo"

# Copy source code
COPY . .

# Build binary using make to include commit and build date
ARG GIT_COMMIT=unknown
RUN make build GIT_COMMIT=${GIT_COMMIT}

# Compile the E2E test binary for ginkgo CLI parallel execution
RUN CGO_ENABLED=0 go test -c -o /build/bin/e2e.test ./e2e

RUN chmod +x /build/bin/hyperfleet-e2e /build/bin/ginkgo /build/bin/e2e.test

FROM registry.ci.openshift.org/ci/hyperfleet-credential-provider:latest AS hyperfleet-credential-provider

# Runtime stage
FROM ${BASE_IMAGE}

# Install runtime dependencies and tools
USER root
RUN dnf -y install --allowerasing jq gettext curl git && dnf clean all

# Tools below are pinned linux/amd64 builds, verified against upstream-published checksums

# Install kubectl
ARG KUBECTL_VERSION=v1.37.1
ARG KUBECTL_SHA256=65691ff77eb6fa44c908b77a1082c9f092c3b9733b5cefabec0d1104890e21a8
RUN curl -fsSL "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/amd64/kubectl" -o /usr/local/bin/kubectl && \
    echo "${KUBECTL_SHA256}  /usr/local/bin/kubectl" | sha256sum -c - && \
    chmod +x /usr/local/bin/kubectl

# Install Helm
ARG HELM_VERSION=v3.22.1
ARG HELM_SHA256=896cf63684a42a723d61c980369d6e588330927deb13676ca0d85006dd6da0ea
RUN curl -fsSL "https://get.helm.sh/helm-${HELM_VERSION}-linux-amd64.tar.gz" -o /tmp/helm.tar.gz && \
    echo "${HELM_SHA256}  /tmp/helm.tar.gz" | sha256sum -c - && \
    tar --no-same-owner -xzf /tmp/helm.tar.gz -C /usr/local/bin --strip-components=1 linux-amd64/helm && \
    rm /tmp/helm.tar.gz

# Set up Helm directories (cache and config writable, plugins read-only)
# OpenShift Prow runs with random UID in group 0, requiring group-writable dirs
RUN mkdir -p /tmp/helm-home/.cache/helm \
            /tmp/helm-home/.config/helm \
            /usr/local/share/helm/plugins && \
    chown -R 0:0 /tmp/helm-home /usr/local/share/helm/plugins && \
    chmod -R g=u /tmp/helm-home /usr/local/share/helm/plugins

ENV HELM_CACHE_HOME=/tmp/helm-home/.cache/helm \
    HELM_CONFIG_HOME=/tmp/helm-home/.config/helm \
    HELM_PLUGINS=/usr/local/share/helm/plugins

# Install Helm plugins (helm-git has no release archive, so its tag is checked against the commit)
ARG HELM_GIT_VERSION=v1.5.2
ARG HELM_GIT_COMMIT=8f910e377bf743cc07ce963a696b1e7929aebb80
ARG HELM_DIFF_VERSION=3.15.7
ARG HELM_DIFF_SHA256=ff7b1b6f3f4fafeac1d0e2c0c82eafac6fa7601bda52396355bf931ecf3a304a
RUN git -c advice.detachedHead=false clone -q --depth 1 --branch ${HELM_GIT_VERSION} \
        https://github.com/aslafy-z/helm-git "${HELM_PLUGINS}/helm-git" && \
    test "$(git -C "${HELM_PLUGINS}/helm-git" rev-parse HEAD)" = "${HELM_GIT_COMMIT}" && \
    curl -fsSL "https://github.com/databus23/helm-diff/releases/download/v${HELM_DIFF_VERSION}/helm-diff-linux-amd64.tgz" -o /tmp/helm-diff.tgz && \
    echo "${HELM_DIFF_SHA256}  /tmp/helm-diff.tgz" | sha256sum -c - && \
    tar --no-same-owner -xzf /tmp/helm-diff.tgz -C "${HELM_PLUGINS}" && \
    rm /tmp/helm-diff.tgz

ARG HELMFILE_VERSION=1.5.2
ARG HELMFILE_SHA256=96e342ff19a45c2e36371f93cf6264f3ef285bbde0fc363e56cb53bdef3df1ff
# Install helmfile
RUN curl -fsSL "https://github.com/helmfile/helmfile/releases/download/v${HELMFILE_VERSION}/helmfile_${HELMFILE_VERSION}_linux_amd64.tar.gz" -o /tmp/helmfile.tar.gz && \
    echo "${HELMFILE_SHA256}  /tmp/helmfile.tar.gz" | sha256sum -c - && \
    tar --no-same-owner -xzf /tmp/helmfile.tar.gz -C /usr/local/bin helmfile && \
    chmod +x /usr/local/bin/helmfile && \
    rm /tmp/helmfile.tar.gz

WORKDIR /e2e

# Copy the hyperfleet-credential-provider binary
COPY --from=hyperfleet-credential-provider /app/hyperfleet-credential-provider /usr/local/bin/

# Copy binaries from builder (main binary + ginkgo CLI + test binary)
COPY --from=builder /build/bin/hyperfleet-e2e /usr/local/bin/
COPY --from=builder /build/bin/ginkgo /usr/local/bin/
COPY --from=builder /build/bin/e2e.test /usr/local/bin/

# Copy test payloads and fixtures
COPY --from=builder /build/testdata /e2e/testdata

# Copy env files
COPY --from=builder /build/env /e2e/env

# Copy cleanup scripts
COPY --from=builder /build/scripts /e2e/scripts

# Copy default config (fallback if ConfigMap is not mounted)
COPY --from=builder /build/configs /e2e/configs

ENTRYPOINT ["/usr/local/bin/hyperfleet-e2e"]
CMD ["test", "--help"]

LABEL name="hyperfleet-e2e" \
      vendor="Red Hat" \
      summary="HyperFleet E2E Testing Framework" \
      description="End to end testing for HyperFleet cluster lifecycle management"
