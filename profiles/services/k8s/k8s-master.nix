{ pkgs, ... }:
let
  kubeMasterIP = "100.93.5.12";
  kubeMasterHostname = "INFINITY";
  kubeMasterAPIServerPort = 6443;
in
{
  networking.extraHosts = ''
    ${kubeMasterIP} ${kubeMasterHostname}
  '';

  environment.systemPackages = with pkgs; [
    kompose
    kubectl
    kubernetes

    (pkgs.writeShellScriptBin "k8s-reset-node"
      # ref: https://github.com/TUM-DSE/doctor-cluster-config/blob/0c70f0c95db6321d45e4182f84a753cf1eabadb3/modules/k3s/k3s-reset-node
      ''
        #!/bin/sh
        # THIS IS DANGEROUS BECAUSE IT DELETES SECRETS/CERTS AND EVERYTHING ELSE FROM K8S.
        set -eux -o pipefail
        shopt -s nullglob
        systemctl stop certmgr containerd kubelet kube-proxy flannel
        find /sys/fs/cgroup/systemd/system.slice/containerd.service* /sys/fs/cgroup/systemd/kubepods* /sys/fs/cgroup/kubepods* -name cgroup.procs | \
            xargs -r cat | xargs -r kill -9
        mount | awk '/\/var\/lib\/kubelet|\/run\/netns|\/run\/containerd/ {print $3}' | xargs -r umount
        dataset=$((btrfs subvolume list / | grep /var/lib/containerd/io.containerd.snapshotter.v1.btrfs /proc/mounts || :) | awk '{print $1}')
        if [[ -n "$dataset" ]]; then
            btrfs subvolume delete "$dataset"
        fi
        rm -rf /var/lib/kubernetes/ /var/lib/etcd/ /var/lib/cfssl/ /var/lib/kubelet/ /etc/kube-flannel/ /etc/kubernetes/ /run/containerd /var/lib/containerd
      ''
    )
  ];

  networking.firewall.allowedTCPPorts = [
    kubeMasterAPIServerPort
    8888 # easycerts iirc
    10250 # kubelet API server
  ];

  services.kubernetes = {
    roles = [ "master" "node" ];
    masterAddress = kubeMasterHostname;
    apiserverAddress = "https://${kubeMasterHostname}:${toString kubeMasterAPIServerPort}";
    easyCerts = true;
    apiserver = {
      securePort = kubeMasterAPIServerPort;
      advertiseAddress = kubeMasterIP;
    };

    addons.dns.enable = true;

    kubelet.extraOpts = "--fail-swap-on=false";
  };

  systemd.services.etcd.preStart = ''${pkgs.writeShellScript "etcd-wait" ''
    while [ ! -f /var/lib/kubernetes/secrets/etcd.pem ]; do sleep 1; done
  ''}'';
}
