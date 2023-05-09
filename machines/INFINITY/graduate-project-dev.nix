{ inputs, ... }:

{
  specialisation = {
    k8s.configuration = {
      imports = [
        inputs.self.nixosProfiles.k8s-master
      ];

      services.kubernetes.kubelet.hostname = "infinity";
    };
  };
}
