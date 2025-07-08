{
  enableACME = true;
  forceSSL = true;
  quic = true;
  kTLS = true;

  extraConfig = ''
    # 0-RTT: Enable TLS 1.3 early data
    ssl_early_data on;
    # Enables sending in optimized batch mode using segmentation offloading.
    quic_gso on;
    # Enables the QUIC Address Validation feature. This includes sending a new
    # token in a Retry packet or a NEW_TOKEN frame and validating a token
    # received in the initial packet.
    quic_retry on;

    # Advertise http3, not done by NixOS option http3=true yet
    add_header Alt-Svc 'h3=":443"; ma=86400';
  '';
}
