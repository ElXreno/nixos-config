{ inputs, ... }:

{
  specialisation = {
    k8s.configuration = {
      imports = [
        inputs.self.nixosProfiles.k8s-master
      ];

      virtualisation.containerd.settings.plugins."io.containerd.grpc.v1.cri".containerd.snapshotter = "btrfs";
      services.kubernetes.kubelet.hostname = "infinity";
    };
  };
}
