FROM registry.ci.openshift.org/openshift/release:golang-1.19 AS builder
WORKDIR /workdir
COPY . .
RUN make build

FROM registry.ci.openshift.org/ocp/4.12:base
COPY --from=builder /workdir/webhook /usr/bin/
COPY --from=builder /workdir//configuration.yaml /etc/setsriovdefault/config/override.yaml
