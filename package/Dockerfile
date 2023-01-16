FROM registry.suse.com/bci/bci-micro:15.4
RUN echo 'helmprojectoperator:x:1000:1000::/home/helmprojectoperator:/bin/bash' >> /etc/passwd && \
    echo 'helmprojectoperator:x:1000:' >> /etc/group && \
    mkdir /home/helmprojectoperator && \
    chown -R helmprojectoperator:helmprojectoperator /home/helmprojectoperator
COPY bin/helm-project-operator /usr/bin/
USER helmprojectoperator
CMD ["helm-project-operator"]
