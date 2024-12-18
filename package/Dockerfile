FROM registry.suse.com/bci/golang:1.22 as builder
RUN zypper -n install git vim less file curl wget helm
WORKDIR /usr/src/app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN make build

FROM registry.suse.com/bci/bci-micro:15.5
RUN echo 'helmprojectoperator:x:1000:1000::/home/helmprojectoperator:/bin/bash' >> /etc/passwd && \
    echo 'helmprojectoperator:x:1000:' >> /etc/group && \
    mkdir /home/helmprojectoperator && \
    chown -R helmprojectoperator:helmprojectoperator /home/helmprojectoperator
USER helmprojectoperator
COPY --from=builder /usr/src/app/bin/helm-project-operator /usr/bin/
CMD ["helm-locker"]