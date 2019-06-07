FROM registry.svc.ci.openshift.org/openshift/release:golang-1.10 AS builder
WORKDIR /go/src/github.com/openshift/cluster-api-provider-kubemark
COPY . .
RUN go build -o ./machine-controller-manager ./cmd/manager
RUN go build -o ./manager ./vendor/sigs.k8s.io/cluster-api/cmd/manager

FROM docker.io/gofed/base:baseci
RUN INSTALL_PKGS=" \
      openssh \
      " && \
    yum install -y $INSTALL_PKGS && \
    rpm -V $INSTALL_PKGS && \
    yum clean all && \
    curl -LO https://storage.googleapis.com/kubernetes-release/release/$(curl -s https://storage.googleapis.com/kubernetes-release/release/stable.txt)/bin/linux/amd64/kubectl && \
    chmod +x ./kubectl && \
    mv ./kubectl /bin/kubectl && \
    curl -LO https://github.com/stedolan/jq/releases/download/jq-1.5/jq-linux64 && \
    chmod +x ./jq-linux64 && \
    mv ./jq-linux64 /bin/jq

COPY --from=builder /go/src/github.com/openshift/cluster-api-provider-kubemark/manager /
COPY --from=builder /go/src/github.com/openshift/cluster-api-provider-kubemark/machine-controller-manager /
